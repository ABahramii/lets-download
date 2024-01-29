package main

import (
	"fmt"
	"let_s_download/utils"
	"os"
	"time"
)

func main() {
	start := time.Now()

	download := utils.Download{
		Url:           "http://127.0.0.1:80/test-movie.mkv",
		TargetPath:    "./",
		TotalSections: 10,
	}

	resourceName, err := utils.ExtractResourceName(download.Url)
	if err != nil {
		fmt.Println("URL is invalid")
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
		panic(err)
	}

	fmt.Printf("Download completed in %v seconds\n", time.Now().Sub(start).Seconds())
}
