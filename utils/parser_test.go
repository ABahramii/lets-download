package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractResourceName(t *testing.T) {
	tests := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{url: "http://a.com/dir/file.zip", want: "file.zip"},
		{url: "http://a.com/file?filename=movie.mkv", want: "movie.mkv"},
		{url: "http://a.com/file?filename=some/dir/movie.mkv", want: "movie.mkv"},
		{url: "http://a.com/file?filename=/", want: "/"},
		{url: "http://a.com/", wantErr: true},
		{url: "http://a.com", wantErr: true},
		{url: "http://a.com/%zz", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got, err := ExtractResourceName(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateTargetPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateTargetPath(dir); err != nil {
		t.Fatalf("directory: unexpected error: %v", err)
	}
	if err := ValidateTargetPath(filepath.Join(dir, "missing")); err == nil || err.Error() != "target path does not exits" {
		t.Fatalf("missing path: got %v", err)
	}
	if err := ValidateTargetPath(file); err == nil || err.Error() != "target path is not a directory" {
		t.Fatalf("file: got %v", err)
	}
}
