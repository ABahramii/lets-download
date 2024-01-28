package main

import (
	"fmt"
	"time"
)

func main() {
	start := time.Now()

	download := Download{
		url:           "http://127.0.0.1:80/test-movie.mkv",
		targetPath:    "./downloads/test-movie.mkv",
		totalSections: 10,
	}

	err := download.Do()
	if err != nil {
		fmt.Println("An error occurred while downloading.")
		panic(err)
	}

	fmt.Printf("Download completed in %v seconds\n", time.Now().Sub(start).Seconds())
}
