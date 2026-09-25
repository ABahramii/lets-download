package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// httpClient has no overall timeout so large downloads aren't cut off. These
// timeouts only cover connecting and waiting for response headers; a section
// whose body stops arriving is aborted by sectionIdleTimeout instead.
var httpClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}

type Download struct {
	URL          string
	TargetPath   string
	ResourceName string
	// TotalSections is how many byte ranges are downloaded in parallel for this
	// file; 0 or less means defaultSectionCount (10).
	TotalSections int
	// Out receives progress messages; nil means os.Stdout.
	// Sections write to it concurrently, so it must be safe for concurrent use.
	Out io.Writer
}

func (download *Download) out() io.Writer {
	if download.Out == nil {
		return os.Stdout
	}
	return download.Out
}

// NewDownload validates rawURL (it must be http or https with a host and contain
// a resource name) and returns a Download of it into targetPath.
func NewDownload(rawURL, targetPath string) (*Download, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errInvalidURL
	}
	resourceName, err := resourceNameFromURL(u)
	if err != nil {
		return nil, err
	}
	return &Download{
		URL:           rawURL,
		TargetPath:    targetPath,
		ResourceName:  resourceName,
		TotalSections: defaultSectionCount,
	}, nil
}

func (download *Download) Do() (err error) {
	fmt.Fprintln(download.out(), "making connection")
	totalSize, err := download.getResourceSize()
	if err != nil {
		return err
	}

	sections := makeSections(download.TotalSections, totalSize)
	// remove temp files even when a section or the merge fails
	defer func() {
		if removeErr := download.removeTempFiles(sections); err == nil {
			err = removeErr
		}
	}()

	err = download.concurrentDownload(sections)
	if err != nil {
		return err
	}

	return mergeFiles(download.TargetPath, download.ResourceName, sections)
}

// ErrDuplicateOutput is returned by DownloadAll for a download whose output file
// is already used by an earlier download in the same batch.
var ErrDuplicateOutput = errors.New("duplicate resource name")

// DownloadAll downloads all given resources concurrently, at most maxParallel
// at a time (maxParallel <= 0 means no limit).
// Downloads that would write to the same output file as an earlier one in the
// list are not started and are reported as errors.
// A failed download does not stop the others; all failures are returned joined.
func DownloadAll(downloads []*Download, maxParallel int) error {
	unique, errs := rejectDuplicateOutputs(downloads)
	runErrs := runParallel(len(unique), maxParallel, func(i int) error {
		if err := unique[i].Do(); err != nil {
			return fmt.Errorf("%s: %w", unique[i].URL, err)
		}
		return nil
	})
	return errors.Join(append(errs, runErrs...)...)
}

// rejectDuplicateOutputs keeps the first download for each output file. Two
// downloads with the same target path and resource name would share temp and
// output files, corrupting each other when run concurrently.
func rejectDuplicateOutputs(downloads []*Download) (unique []*Download, errs []error) {
	firstURL := make(map[string]string, len(downloads))
	for _, download := range downloads {
		key := outputFilePath(download.TargetPath, download.ResourceName)
		if url, ok := firstURL[key]; ok {
			errs = append(errs, fmt.Errorf("%s: %w %q, already used by %s", download.URL, ErrDuplicateOutput, download.ResourceName, url))
			continue
		}
		firstURL[key] = download.URL
		unique = append(unique, download)
	}
	return unique, errs
}

func (download *Download) getResourceSize() (int, error) {
	request, err := download.getNewRequest(http.MethodHead)
	if err != nil {
		return 0, err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	fmt.Fprintf(download.out(), "status: %v\n", response.StatusCode)

	if response.StatusCode > 299 {
		return 0, fmt.Errorf("can't process, response code is %d", response.StatusCode)
	}

	totalSize := response.Header.Get("Content-Length")
	fmt.Fprintf(download.out(), "file: %s\nsize: %s bytes\n", download.ResourceName, totalSize)
	return strconv.Atoi(totalSize)
}

func (download *Download) getNewRequest(method string) (*http.Request, error) {
	request, err := http.NewRequest(
		method,
		download.URL,
		nil,
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "let's-download")
	return request, nil
}

func (download *Download) concurrentDownload(sections []byteRange) error {
	errs := runParallel(len(sections), 0, func(i int) error {
		if err := download.downloadSection(i, sections[i]); err != nil {
			return fmt.Errorf("failed to download section %d: %w", i, err)
		}
		return nil
	})
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (download *Download) downloadSection(index int, section byteRange) error {
	ctx, watchdog := withIdleTimeout(context.Background(), sectionIdleTimeout)
	defer watchdog.stop()

	request, err := download.getNewRequest(http.MethodGet)
	if err != nil {
		return err
	}
	request = request.WithContext(ctx)
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", section.start, section.end))
	response, err := httpClient.Do(request)
	if err != nil {
		return watchdog.err(err)
	}
	defer response.Body.Close()

	// anything other than 206 (e.g. 429, 503 or a full-body 200) must not be merged into the file
	if response.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("unexpected response code %d for range request", response.StatusCode)
	}
	// a 206 for a different range (e.g. a server that caps chunk sizes) must not be merged either
	if value := response.Header.Get("Content-Range"); value != "" {
		got, err := parseContentRange(value)
		if err != nil {
			return err
		}
		if got != section {
			return fmt.Errorf("server sent bytes %d-%d, requested %d-%d", got.start, got.end, section.start, section.end)
		}
	}

	file, err := os.Create(sectionFilePath(download.TargetPath, download.ResourceName, index))
	if err != nil {
		return err
	}
	defer file.Close()

	// read at most one byte more than expected, enough to detect a body that is too long
	n, err := io.Copy(file, io.LimitReader(watchdog.reader(response.Body), int64(section.length())+1))
	if err != nil {
		return watchdog.err(err)
	}
	if want := int64(section.length()); n > want {
		return fmt.Errorf("received more than %d bytes for range %d-%d", want, section.start, section.end)
	} else if n < want {
		return fmt.Errorf("received %d bytes for range %d-%d, want %d", n, section.start, section.end, want)
	}
	fmt.Fprintf(download.out(), "downloaded %d bytes from section %d: [%d %d]\n", n, index, section.start, section.end)

	return file.Close()
}
