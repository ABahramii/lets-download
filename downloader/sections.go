package downloader

import (
	"fmt"
	"strconv"
	"strings"
)

// defaultSectionCount is how many byte ranges a file is split into when
// Download.TotalSections is not set.
const defaultSectionCount = 10

// byteRange is an inclusive range of bytes, as used in an HTTP Range header.
type byteRange struct {
	start int
	end   int
}

func (r byteRange) length() int {
	return r.end - r.start + 1
}

// parseContentRange parses a Content-Range header value such as
// "bytes 0-99/1000" (the total may be "*").
func parseContentRange(value string) (byteRange, error) {
	invalid := fmt.Errorf("invalid Content-Range %q", value)
	rest, ok := strings.CutPrefix(value, "bytes ")
	if !ok {
		return byteRange{}, invalid
	}
	rest, _, ok = strings.Cut(rest, "/")
	if !ok {
		return byteRange{}, invalid
	}
	startStr, endStr, ok := strings.Cut(rest, "-")
	if !ok {
		return byteRange{}, invalid
	}
	start, err := strconv.Atoi(startStr)
	if err != nil {
		return byteRange{}, invalid
	}
	end, err := strconv.Atoi(endStr)
	if err != nil {
		return byteRange{}, invalid
	}
	return byteRange{start: start, end: end}, nil
}

// makeSections splits totalSize bytes into inclusive ranges that together cover
// bytes 0 to totalSize-1: totalSections ranges (defaultSectionCount if
// totalSections <= 0), or one per byte for files smaller than that, and none for
// an empty file. The last range also takes the remainder.
func makeSections(totalSections, totalSize int) []byteRange {
	if totalSections <= 0 {
		totalSections = defaultSectionCount
	}
	count := min(totalSections, totalSize)
	if count <= 0 {
		return nil
	}

	sectionSize := totalSize / count
	sections := make([]byteRange, count)
	for i := range sections {
		start := i * sectionSize
		end := start + sectionSize - 1
		if i == count-1 {
			end = totalSize - 1
		}
		sections[i] = byteRange{start: start, end: end}
	}
	return sections
}
