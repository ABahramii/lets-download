package downloader

// sectionCount is how many byte ranges a file is split into.
const sectionCount = 10

// byteRange is an inclusive range of bytes, as used in an HTTP Range header.
type byteRange struct {
	start int
	end   int
}

func makeSections(totalSections, totalSize int) []byteRange {
	sections := make([]byteRange, totalSections)

	sectionSize := totalSize / sectionCount
	remain := totalSize % sectionCount
	start := 0
	var end int

	for i := 0; i < sectionCount; i++ {
		if i == sectionCount-1 {
			end = start + sectionSize + remain
		} else {
			end = start + sectionSize - 1
		}
		sections[i] = byteRange{start: start, end: end}
		start = end + 1
	}
	return sections
}
