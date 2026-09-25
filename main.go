package main

import (
	"flag"
	"fmt"
	"let_s_download/utils"
	"os"
	"time"
)

var (
	currentDir, _ = os.Getwd()
	url           = flag.String("url", "http://127.0.0.1:80/test_file", "URL for download file")
	targetPath    = flag.String("targetPath", currentDir, "path for downloaded file")
	filePath      = flag.String("f", "", "path of a file containing one download link per line")
)

func main() {
	start := time.Now()
	flag.Parse()

	err := utils.ValidateTargetPath(*targetPath)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(0)
	}

	if *filePath != "" {
		downloadFromFile(*filePath, *targetPath)
	} else {
		downloadSingle(*url, *targetPath)
	}

	fmt.Printf("Download completed in %v seconds\n", time.Now().Sub(start).Seconds())
}

func downloadSingle(url, targetPath string) {
	download, err := newDownload(url, targetPath)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(0)
	}

	err = download.Do()
	if err != nil {
		fmt.Println("An error occurred while downloading.")
		// Todo: remove panic
		panic(err)
	}
}

func downloadFromFile(filePath, targetPath string) {
	links, err := utils.ReadLinks(filePath)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	var downloads []*utils.Download
	for _, link := range links {
		download, err := newDownload(link, targetPath)
		if err != nil {
			fmt.Printf("skipping %s: %v\n", link, err)
			continue
		}
		downloads = append(downloads, download)
	}

	err = utils.DownloadAll(downloads)
	if err != nil {
		fmt.Println("Some downloads failed:")
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func newDownload(url, targetPath string) (*utils.Download, error) {
	resourceName, err := utils.ExtractResourceName(url)
	if err != nil {
		return nil, err
	}
	return &utils.Download{
		URL:           url,
		TargetPath:    targetPath,
		ResourceName:  resourceName,
		TotalSections: 10,
	}, nil
}
