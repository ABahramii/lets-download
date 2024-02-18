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
)

func main() {
	start := time.Now()
	flag.Parse()

	download := utils.Download{
		URL:           *url,
		TargetPath:    *targetPath,
		TotalSections: 10,
	}

	resourceName, err := utils.ExtractResourceName(download.URL)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(0)
	}
	download.ResourceName = resourceName

	err = utils.ValidateTargetPath(download.TargetPath)
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

	fmt.Printf("Download completed in %v seconds\n", time.Now().Sub(start).Seconds())
}
