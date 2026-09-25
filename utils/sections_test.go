package utils

import (
	"reflect"
	"testing"
)

func TestMakeSections(t *testing.T) {
	tests := []struct {
		name      string
		totalSize int
		want      [][2]int
	}{
		{
			name:      "evenly divisible",
			totalSize: 1000,
			want: [][2]int{
				{0, 99}, {100, 199}, {200, 299}, {300, 399}, {400, 499},
				{500, 599}, {600, 699}, {700, 799}, {800, 899}, {900, 1000},
			},
		},
		{
			// the remainder goes to the last section; its end is totalSize (one past the
			// last byte), which works only because servers clamp the range
			name:      "with remainder",
			totalSize: 1005,
			want: [][2]int{
				{0, 99}, {100, 199}, {200, 299}, {300, 399}, {400, 499},
				{500, 599}, {600, 699}, {700, 799}, {800, 899}, {900, 1005},
			},
		},
		{
			// current behavior for files smaller than 10 bytes: invalid ranges
			name:      "smaller than section count",
			totalSize: 5,
			want: [][2]int{
				{0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1},
				{0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, 5},
			},
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
