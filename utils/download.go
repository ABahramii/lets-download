package utils

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// httpClient has no overall timeout so large downloads aren't cut off,
// but a server that stops responding won't hang the download forever.
var httpClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}

// sectionCount is how many byte ranges a file is split into.
const sectionCount = 10

// byteRange is an inclusive range of bytes, as used in an HTTP Range header.
type byteRange struct {
	start int
	end   int
}

type Download struct {
	URL           string
	TargetPath    string
	ResourceName  string
	TotalSections int
}

func (download *Download) Do() (err error) {
	fmt.Println("making connection")
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

// DownloadAll downloads all given resources concurrently, at most maxParallel
// at a time (maxParallel <= 0 means no limit).
// A failed download does not stop the others; all failures are returned joined.
func DownloadAll(downloads []*Download, maxParallel int) error {
	errs := runParallel(len(downloads), maxParallel, func(i int) error {
		if err := downloads[i].Do(); err != nil {
			return fmt.Errorf("%s: %w", downloads[i].URL, err)
		}
		return nil
	})
	return errors.Join(errs...)
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
	fmt.Printf("status: %v\n", response.StatusCode)

	if response.StatusCode > 299 {
		return 0, fmt.Errorf("can't process, response code is %d", response.StatusCode)
	}

	totalSize := response.Header.Get("Content-Length")
	fmt.Printf("file: %s\nsize: %s bytes\n", download.ResourceName, totalSize)
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
			return fmt.Errorf("failed to download section %download: %w", i, err)
		}
		return nil
	})
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (download *Download) downloadSection(index int, section byteRange) error {
	request, err := download.getNewRequest(http.MethodGet)
	if err != nil {
		return err
	}
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", section.start, section.end))
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// anything other than 206 (e.g. 429, 503 or a full-body 200) must not be merged into the file
	if response.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("unexpected response code %d for range request", response.StatusCode)
	}

	file, err := os.Create(sectionFilePath(download.TargetPath, download.ResourceName, index))
	if err != nil {
		return err
	}
	defer file.Close()

	n, err := io.Copy(file, response.Body)
	if err != nil {
		return err
	}
	fmt.Printf("downloaded %d bytes from section %d: [%d %d]\n", n, index, section.start, section.end)

	return file.Close()
}

func (download *Download) removeTempFiles(sections []byteRange) error {
	for i := range sections {
		err := os.Remove(sectionFilePath(download.TargetPath, download.ResourceName, i))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// sectionFilePath includes the resource name so concurrent downloads
// into the same directory don't overwrite each other's temp files.
func sectionFilePath(targetPath, resourceName string, i int) string {
	return filepath.Join(targetPath, fmt.Sprintf("%s.section-%d.tmp", resourceName, i))
}

func makeSections(totalSections, totalSize int) []byteRange {
	sections := make([]byteRange, totalSections)

	sectionSize := totalSize / sectionCount
	remain := totalSize % sectionCount
	start := 0
	var end int

	for i := 0; i < sectionCount; i++ {
		if i == sectionCount-1 {
			end = start + sectionSize + remain
		} else {
			end = start + sectionSize - 1
		}
		sections[i] = byteRange{start: start, end: end}
		start = end + 1
	}
	return sections
}

func mergeFiles(targetPath, resourceName string, sections []byteRange) error {
	filePath := filepath.Join(targetPath, resourceName+".mp4")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()
	for i := range sections {
		if err := appendFile(file, sectionFilePath(targetPath, resourceName, i)); err != nil {
			return err
		}
	}
	return nil
}

// appendFile streams the file at path into dst without loading it into memory.
func appendFile(dst io.Writer, path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()

	_, err = io.Copy(dst, src)
	return err
}
