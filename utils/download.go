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
	"sync"
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
	if maxParallel <= 0 {
		maxParallel = len(downloads)
	}
	var wg sync.WaitGroup
	wg.Add(len(downloads))
	errorsCh := make(chan error, len(downloads))
	semaphore := make(chan struct{}, maxParallel)

	for _, download := range downloads {
		go func(download *Download) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			if err := download.Do(); err != nil {
				errorsCh <- fmt.Errorf("%s: %w", download.URL, err)
			}
		}(download)
	}

	wg.Wait()
	close(errorsCh)

	var errs []error
	for err := range errorsCh {
		errs = append(errs, err)
	}
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

func (download *Download) concurrentDownload(sections [][2]int) error {
	var wg sync.WaitGroup
	wg.Add(len(sections))
	errorsCh := make(chan error, len(sections))

	for i, section := range sections {
		go func(i int, section [2]int) {
			defer wg.Done()
			if err := download.downloadSection(i, section); err != nil {
				errorsCh <- fmt.Errorf("failed to download section %download: %w", i, err)
				return
			}
		}(i, section)
	}

	wg.Wait()
	close(errorsCh)

	for err := range errorsCh {
		if err != nil {
			return err
		}
	}

	return nil
}

func (download *Download) downloadSection(index int, section [2]int) error {
	request, err := download.getNewRequest(http.MethodGet)
	if err != nil {
		return err
	}
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", section[0], section[1]))
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
	fmt.Printf("downloaded %d bytes from section %d: %d\n", n, index, section)

	return file.Close()
}

func (download *Download) removeTempFiles(sections [][2]int) error {
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

func makeSections(totalSections, totalSize int) [][2]int {
	sections := make([][2]int, totalSections)

	sectionSize := totalSize / 10
	remain := totalSize % 10
	start := 0
	var end int

	for i := 0; i < 10; i++ {
		if i == 9 {
			end = start + sectionSize + remain
		} else {
			end = start + sectionSize - 1
		}
		sections[i][0] = start
		sections[i][1] = end
		start = end + 1
	}
	return sections
}

func mergeFiles(targetPath, resourceName string, sections [][2]int) error {
	filePath := filepath.Join(targetPath, resourceName+".mp4")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()
	for i := range sections {
		b, err := os.ReadFile(sectionFilePath(targetPath, resourceName, i))
		if err != nil {
			return err
		}
		_, err = file.Write(b)
		if err != nil {
			return err
		}
	}
	return nil
}
