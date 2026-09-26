package main

import (
	"context"
	"flag"
	"fmt"
	"let_s_download/downloader"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	currentDir, _  = os.Getwd()
	urlFlag        = flag.String("url", "http://127.0.0.1:80/test_file", "URL for download file")
	targetPathFlag = flag.String("targetPath", currentDir, "path for downloaded file")
	linksFileFlag  = flag.String("f", "", "path of a file containing one download link per line (overrides -url)")
	progressFlag   = flag.Bool("progress", true, "show progress bars (only when stdout is a terminal)")
)

const maxParallelDownloads = 4

func main() {
	start := time.Now()
	flag.Parse()

	err := downloader.ValidateTargetPath(*targetPathFlag)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(0)
	}

	// Ctrl-C cancels the downloads so they can remove their part files.
	// After the first signal the default handling is restored, so a second
	// Ctrl-C kills the program immediately.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()

	if *linksFileFlag != "" {
		downloadFromFile(ctx, *linksFileFlag, *targetPathFlag)
	} else {
		downloadSingle(ctx, *urlFlag, *targetPathFlag)
	}

	fmt.Printf("Download completed in %v seconds\n", time.Now().Sub(start).Seconds())
}

func downloadSingle(ctx context.Context, url, targetPath string) {
	download, err := downloader.NewDownload(url, targetPath)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(0)
	}

	bars := newProgressBars()
	bars.track(download)
	err = download.DoContext(ctx)
	bars.wait()
	exitIfInterrupted(ctx, err)
	if err != nil {
		fmt.Println("An error occurred while downloading:")
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func downloadFromFile(ctx context.Context, filePath, targetPath string) {
	links, err := downloader.ReadLinks(filePath)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	var downloads []*downloader.Download
	seen := make(map[string]bool, len(links))
	for _, link := range links {
		if seen[link] {
			fmt.Printf("skipping %s: duplicate link\n", link)
			continue
		}
		seen[link] = true

		download, err := downloader.NewDownload(link, targetPath)
		if err != nil {
			fmt.Printf("skipping %s: %v\n", link, err)
			continue
		}
		downloads = append(downloads, download)
	}
	if len(downloads) == 0 {
		fmt.Println("no valid links found")
		os.Exit(1)
	}

	bars := newProgressBars()
	for _, download := range downloads {
		bars.track(download)
	}
	err = downloader.DownloadAllContext(ctx, downloads, maxParallelDownloads)
	bars.wait()
	exitIfInterrupted(ctx, err)
	if err != nil {
		fmt.Println("Some downloads failed:")
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

// exitIfInterrupted exits with the conventional status 130 (128 + SIGINT) when
// the downloads failed because a signal cancelled them, instead of listing each
// cancelled request as an error.
func exitIfInterrupted(ctx context.Context, err error) {
	if err != nil && ctx.Err() != nil {
		fmt.Println("Download interrupted.")
		os.Exit(130)
	}
}
