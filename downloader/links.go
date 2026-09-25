package downloader

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ReadLinks reads a file containing one download link per line.
// Empty lines and lines starting with '#' are ignored.
func ReadLinks(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("can't open links file: %w", err)
	}
	defer file.Close()

	var links []string
	scanner := bufio.NewScanner(file)
	// signed URLs can be longer than the default 64KB line limit
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		links = append(links, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("can't read links file: %w", err)
	}

	if len(links) == 0 {
		return nil, errors.New("no links found in file")
	}
	return links, nil
}
