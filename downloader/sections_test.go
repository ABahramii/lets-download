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

// every size must be covered exactly: ranges are valid, contiguous, start at 0
// and end at the last byte
func TestMakeSections_CoversEveryByte(t *testing.T) {
	sizes := []int{19, 20, 21, 99, 101, 999, 1001, 123457}
	for size := 1; size <= 12; size++ {
		sizes = append(sizes, size)
	}

	for _, size := range sizes {
		sections := makeSections(10, size)
		if want := min(sectionCount, size); len(sections) != want {
			t.Fatalf("size %d: got %d sections, want %d", size, len(sections), want)
		}
		next := 0
		for i, section := range sections {
			if section.start != next || section.end < section.start {
				t.Fatalf("size %d: invalid section %d %v (expected start %d)", size, i, section, next)
			}
			next = section.end + 1
		}
		if next != size {
			t.Fatalf("size %d: sections end at byte %d, want %d", size, next-1, size-1)
		}
	}
}
