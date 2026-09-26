package downloader

import "io"

// Progress receives the progress of one Download. Add is called concurrently
// from every section, so implementations must be safe for concurrent use.
type Progress interface {
	// Start is called once the file is split into sections, with the size of
	// each section. The size of the file is their sum.
	Start(sectionSizes []int64)
	// Add is called with the number of bytes just received for a section.
	Add(section, n int)
	// Finish is called once when Do returns, with its error (nil on success).
	Finish(err error)
}

// progressReader wraps r so that every read that returns data is reported to
// progress as bytes of the given section.
func progressReader(r io.Reader, progress Progress, section int) io.Reader {
	if progress == nil {
		return r
	}
	return readerFunc(func(p []byte) (int, error) {
		n, err := r.Read(p)
		if n > 0 {
			progress.Add(section, n)
		}
		return n, err
	})
}
