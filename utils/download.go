package utils

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
)

type Download struct {
	URL           string
	TargetPath    string
	ResourceName  string
	TotalSections int
}

func (download *Download) Do() error {
	fmt.Println("making connection")
	totalSize, err := download.getResourceSize()
	if err != nil {
		return err
	}

	sections := makeSections(download.TotalSections, totalSize)
	err = download.concurrentDownload(sections)
	if err != nil {
		return err
	}

	err = mergeFiles(download.TargetPath, download.ResourceName, sections)
	if err != nil {
		return err
	}

	err = download.removeTempFiles(sections)
	if err != nil {
		return err
	}

	return nil
}

func (download *Download) getResourceSize() (int, error) {
	response, err := download.requestResourceSize()
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	fmt.Printf("Got %v\n", response.StatusCode)

	if response.StatusCode > 299 {
		return 0, errors.New(fmt.Sprintf("can't process, response code is %d", response.StatusCode))
	}

	totalSize := response.Header.Get("Content-Length")
	if err != nil {
		return 0, err
	}
	fmt.Printf("size is %d bytes\n", totalSize)
	return strconv.Atoi(totalSize)
}

func (download *Download) requestResourceSize() (*http.Response, error) {
	request, err := download.getNewRequest(http.MethodHead)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	return response, nil
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

func (download *Download) downloadSection(offset int, section [2]int) error {
	request, err := download.getNewRequest(http.MethodGet)
	if err != nil {
		return err
	}
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", section[0], section[1]))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}

	b, err := io.ReadAll(response.Body)
	fmt.Printf("downloaded %v bytes from section %d: %d\n", response.Header.Get("Content-Length"), offset, section)
	if err != nil {
		return err
	}

	err = os.WriteFile(fmt.Sprintf("%s/section-%d.tmp", download.TargetPath, offset), b, os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}

func (download *Download) removeTempFiles(sections [][2]int) error {
	for i := range sections {
		err := os.Remove(fmt.Sprintf("%s/section-%d.tmp", download.TargetPath, i))
		if err != nil {
			return err
		}
	}
	return nil
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
	filePath := fmt.Sprintf("%s/%s", targetPath, resourceName)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()
	for i := range sections {
		b, err := os.ReadFile(fmt.Sprintf("%s/section-%d.tmp", targetPath, i))
		if err != nil {
			return err
		}
		n, err := file.Write(b)
		if err != nil {
			return err
		}
		fmt.Printf("%v bytes merged\n", n)
	}
	return nil
}
