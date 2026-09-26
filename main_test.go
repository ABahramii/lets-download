package main

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

var ansiCode = regexp.MustCompile("\x1b\\[[0-9;]*m")

func TestRenderSegments(t *testing.T) {
	tests := []struct {
		name  string
		sizes []int64
		done  []int64
		width int
		want  string
	}{
		{"none done", []int64{10, 10, 10}, []int64{0, 0, 0}, 11, "░░░│░░░│░░░"},
		{"all done", []int64{10, 10, 10}, []int64{10, 10, 10}, 11, "███│███│███"},
		{"one section done", []int64{10, 10, 10}, []int64{0, 10, 0}, 11, "░░░│███│░░░"},
		{"partial sections", []int64{10, 10}, []int64{5, 9}, 9, "██░░│███▌"},
		{"small progress shows a partial cell", []int64{100}, []int64{11}, 5, "▌░░░░"},
		{"proportional segments", []int64{30, 10}, []int64{0, 0}, 9, "░░░░░░│░░"},
		{"narrow width uses one bar", []int64{10, 10, 10}, []int64{10, 10, 0}, 6, "████░░"},
		{"empty section", []int64{0, 10}, []int64{0, 0}, 7, "│░░░░░░"},
		{"no sections", nil, nil, 4, "████"},
		{"zero width", []int64{10}, []int64{5}, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderSegments(tt.sizes, tt.done, tt.width, false, false)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if n := utf8.RuneCountInString(got); n != tt.width {
				t.Errorf("visible width %d, want %d", n, tt.width)
			}

			// with colour, empty cells are spaces on a background colour
			colored := ansiCode.ReplaceAllString(renderSegments(tt.sizes, tt.done, tt.width, false, true), "")
			if want := strings.ReplaceAll(tt.want, cellEmpty, " "); colored != want {
				t.Errorf("colored: got %q, want %q", colored, want)
			}
		})
	}
}

func TestRenderSegments_Colors(t *testing.T) {
	sizes, done := []int64{10, 10, 10}, []int64{10, 4, 0} // section 1: 2 cells and 3/8

	plain := renderSegments(sizes, done, 20, false, false)
	if strings.Contains(plain, "\x1b") {
		t.Errorf("got escape codes with color off: %q", plain)
	}

	colored := renderSegments(sizes, done, 20, false, true)
	if !strings.Contains(colored, colorGreen+cellFilled) || !strings.Contains(colored, colorCyan+cellFilled) {
		t.Errorf("want a green complete and a cyan active segment: %q", colored)
	}
	// the partial cell is drawn on the track background, leaving no gap
	if !strings.Contains(colored, colorCyan+bgTrack+"▍") {
		t.Errorf("want the partial cell on the track background: %q", colored)
	}

	aborted := renderSegments(sizes, done, 20, true, true)
	if strings.Contains(aborted, colorGreen) || strings.Contains(aborted, colorCyan) {
		t.Errorf("want only red filled cells when aborted: %q", aborted)
	}
}
