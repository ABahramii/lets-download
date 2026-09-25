package downloader

import (
	"reflect"
	"testing"
)

func TestMakeSections(t *testing.T) {
	tests := []struct {
		name      string
		totalSize int
		want      []byteRange
	}{
		{
			name:      "evenly divisible",
			totalSize: 1000,
			want: []byteRange{
				{0, 99}, {100, 199}, {200, 299}, {300, 399}, {400, 499},
				{500, 599}, {600, 699}, {700, 799}, {800, 899}, {900, 999},
			},
		},
		{
			name:      "remainder goes to the last section",
			totalSize: 1005,
			want: []byteRange{
				{0, 99}, {100, 199}, {200, 299}, {300, 399}, {400, 499},
				{500, 599}, {600, 699}, {700, 799}, {800, 899}, {900, 1004},
			},
		},
		{
			name:      "smaller than section count",
			totalSize: 5,
			want:      []byteRange{{0, 0}, {1, 1}, {2, 2}, {3, 3}, {4, 4}},
		},
		{
			name:      "one byte",
			totalSize: 1,
			want:      []byteRange{{0, 0}},
		},
		{
			name:      "empty",
			totalSize: 0,
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeSections(10, tt.totalSize)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMakeSections_UsesTotalSections(t *testing.T) {
	tests := []struct {
		name          string
		totalSections int
		totalSize     int
		want          []byteRange
	}{
		{name: "one section", totalSections: 1, totalSize: 100, want: []byteRange{{0, 99}}},
		{name: "three sections", totalSections: 3, totalSize: 100, want: []byteRange{{0, 32}, {33, 65}, {66, 99}}},
		{name: "more sections than bytes", totalSections: 25, totalSize: 3, want: []byteRange{{0, 0}, {1, 1}, {2, 2}}},
		{name: "zero means default", totalSections: 0, totalSize: 20, want: []byteRange{
			{0, 1}, {2, 3}, {4, 5}, {6, 7}, {8, 9}, {10, 11}, {12, 13}, {14, 15}, {16, 17}, {18, 19},
		}},
		{name: "negative means default", totalSections: -1, totalSize: 10, want: []byteRange{
			{0, 0}, {1, 1}, {2, 2}, {3, 3}, {4, 4}, {5, 5}, {6, 6}, {7, 7}, {8, 8}, {9, 9},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeSections(tt.totalSections, tt.totalSize)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// every size must be covered exactly, for any section count: ranges are valid,
// contiguous, start at 0 and end at the last byte
func TestMakeSections_CoversEveryByte(t *testing.T) {
	sizes := []int{19, 20, 21, 99, 101, 999, 1001, 123457}
	for size := 1; size <= 12; size++ {
		sizes = append(sizes, size)
	}

	for _, totalSections := range []int{1, 2, 3, 7, 10, 16, 64} {
		for _, size := range sizes {
			sections := makeSections(totalSections, size)
			if want := min(totalSections, size); len(sections) != want {
				t.Fatalf("sections %d, size %d: got %d sections, want %d", totalSections, size, len(sections), want)
			}
			next := 0
			for i, section := range sections {
				if section.start != next || section.end < section.start {
					t.Fatalf("sections %d, size %d: invalid section %d %v (expected start %d)", totalSections, size, i, section, next)
				}
				next = section.end + 1
			}
			if next != size {
				t.Fatalf("sections %d, size %d: sections end at byte %d, want %d", totalSections, size, next-1, size-1)
			}
		}
	}
}
