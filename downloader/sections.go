package downloader

// sectionCount is how many byte ranges a file is split into.
const sectionCount = 10

// byteRange is an inclusive range of bytes, as used in an HTTP Range header.
type byteRange struct {
	start int
	end   int
}

// makeSections splits totalSize bytes into inclusive ranges that together cover
// bytes 0 to totalSize-1: sectionCount ranges, or one per byte for files smaller
// than that, and none for an empty file. The last range also takes the remainder.
// TODO: totalSections is ignored; sectionCount is always used.
func makeSections(totalSections, totalSize int) []byteRange {
	count := min(sectionCount, totalSize)
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
