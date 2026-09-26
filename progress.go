package main

import (
	"io"
	"let_s_download/downloader"
	"os"
	"strings"
	"sync/atomic"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// ANSI SGR codes used to colour the bars.
const (
	colorReset   = "\x1b[0m"
	colorBold    = "\x1b[1m"
	colorRed     = "\x1b[31m"
	colorGreen   = "\x1b[32m"
	colorYellow  = "\x1b[33m"
	colorMagenta = "\x1b[35m"
	colorCyan    = "\x1b[36m"
	colorGray    = "\x1b[90m"
	// bgTrack is the dark gray background of the unfilled part of a bar.
	bgTrack = "\x1b[48;5;238m"
)

// Characters of a segmented bar.
const (
	cellFilled    = "█"
	cellEmpty     = "░"
	cellSeparator = "│"
)

// partialCells[i] is a cell filled i eighths from the left, so a segment only
// a few cells wide still shows small amounts of progress.
var partialCells = [8]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// progressBars shows one progress bar per download. A nil *progressBars shows
// nothing, and downloads keep printing their text messages instead.
type progressBars struct {
	container *mpb.Progress
	color     bool
}

// newProgressBars returns nil when bars are disabled or stdout isn't a terminal.
// Colours are turned off when the NO_COLOR environment variable is set.
func newProgressBars() *progressBars {
	if !*progressFlag || !isTerminal(os.Stdout) {
		return nil
	}
	return &progressBars{
		container: mpb.New(mpb.WithWidth(60)),
		color:     os.Getenv("NO_COLOR") == "",
	}
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// track gives download a progress bar and silences its text messages, which
// would break the bar rendering.
func (p *progressBars) track(download *downloader.Download) {
	if p == nil {
		return
	}
	download.Out = io.Discard
	download.Progress = &progressBar{container: p.container, name: download.ResourceName, color: p.color}
}

// wait blocks until all bars have been drawn for the last time.
func (p *progressBars) wait() {
	if p != nil {
		p.container.Wait()
	}
}

// progressBar implements downloader.Progress. Its bar is added on Start, once
// the section sizes are known. The bar is split into one segment per section,
// while the statistics next to it are for the whole file.
type progressBar struct {
	container *mpb.Progress
	name      string
	color     bool
	bar       *mpb.Bar
	sizes     []int64
	done      []atomic.Int64 // bytes received per section
}

func (b *progressBar) Start(sectionSizes []int64) {
	b.sizes = sectionSizes
	b.done = make([]atomic.Int64, len(sectionSizes))
	var total int64
	for _, size := range sectionSizes {
		total += size
	}

	b.bar = b.container.MustAdd(total, mpb.BarFillerFunc(b.fill),
		mpb.PrependDecorators(
			decor.Meta(decor.Name(b.name, decor.WCSyncSpaceR), b.paintFunc(colorBold)),
			decor.CountersKibiByte("% .1f / % .1f", decor.WCSyncSpace),
		),
		mpb.AppendDecorators(
			decor.OnAbortMeta(
				decor.OnAbort(decor.Meta(decor.Percentage(decor.WCSyncSpace), b.paintFunc(colorYellow)), "failed"),
				b.paintFunc(colorRed),
			),
			decor.OnCompleteOrOnAbort(decor.Meta(decor.AverageSpeed(decor.SizeB1024(0), "% .1f", decor.WCSyncSpace), b.paintFunc(colorMagenta)), ""),
			decor.OnCompleteMeta(
				decor.OnCompleteOrOnAbort(decor.AverageETA(decor.ET_STYLE_GO, decor.WCSyncSpace), "done"),
				b.paintFunc(colorGreen),
			),
		),
	)
}

func (b *progressBar) Add(section, n int) {
	b.done[section].Add(int64(n))
	b.bar.IncrBy(n)
}

func (b *progressBar) Finish(err error) {
	if b.bar == nil {
		return // failed before the size was known
	}
	if err != nil {
		b.bar.Abort(false)
		return
	}
	// completes the bar even when no bytes were counted, e.g. for an empty file
	b.bar.SetTotal(-1, true)
}

// fill draws the segmented bar. mpb calls it from its render goroutine.
func (b *progressBar) fill(w io.Writer, st decor.Statistics) error {
	width := st.AvailableWidth
	if st.RequestedWidth > 0 && st.RequestedWidth < width {
		width = st.RequestedWidth
	}
	done := make([]int64, len(b.done))
	for i := range b.done {
		done[i] = b.done[i].Load()
	}
	_, err := io.WriteString(w, renderSegments(b.sizes, done, width, st.Aborted, b.color))
	return err
}

func (b *progressBar) paintFunc(code string) func(string) string {
	return func(s string) string { return paint(s, code, b.color) }
}

func paint(s, code string, color bool) string {
	if !color || s == "" {
		return s
	}
	return code + s + colorReset
}

// renderSegments draws a bar of width cells with one segment per section,
// separated by │. Each segment's size is proportional to its section and is
// filled, to an eighth of a cell, as far as that section has been received: cyan while downloading,
// green when complete, red for all sections if aborted. When width is too
// small to give each section at least two cells, it draws a single bar for
// the whole file instead.
func renderSegments(sizes, done []int64, width int, aborted, color bool) string {
	if width <= 0 {
		return ""
	}
	if len(sizes) == 0 || width < 3*len(sizes)-1 {
		var totalSize, totalDone int64
		for i := range sizes {
			totalSize += sizes[i]
			totalDone += min(done[i], sizes[i])
		}
		sizes, done = []int64{totalSize}, []int64{totalDone}
	}

	var totalSize int64
	for _, size := range sizes {
		totalSize += size
	}
	cells := width - (len(sizes) - 1) // without separators

	var sb strings.Builder
	var cumSize int64
	start := 0
	for i, size := range sizes {
		// cumulative rounding makes the segment widths add up to exactly cells
		cumSize += size
		end := cells * (i + 1) / len(sizes)
		if totalSize > 0 {
			end = int((int64(cells)*cumSize + totalSize/2) / totalSize)
		}
		segment := end - start
		start = end

		complete := done[i] >= size
		eighths := segment * 8
		if !complete {
			eighths = int(int64(segment) * 8 * done[i] / size)
		}
		full, partial := eighths/8, partialCells[eighths%8]
		empty := segment - full
		if partial != "" {
			empty--
		}

		fillColor := colorCyan
		if aborted {
			fillColor = colorRed
		} else if complete {
			fillColor = colorGreen
		}
		if i > 0 {
			sb.WriteString(paint(cellSeparator, colorGray, color))
		}
		if !color {
			sb.WriteString(strings.Repeat(cellFilled, full) + partial + strings.Repeat(cellEmpty, empty))
			continue
		}
		// The unfilled part is drawn as a solid background, including the
		// right side of the partial cell, so no black gap shows between the
		// filled and the empty part.
		sb.WriteString(paint(strings.Repeat(cellFilled, full), fillColor, true))
		if track := strings.Repeat(" ", empty); partial != "" || track != "" {
			sb.WriteString(fillColor + bgTrack + partial + track + colorReset)
		}
	}
	return sb.String()
}
