package downloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreatePartFile(t *testing.T) {
	path := partFilePath(t.TempDir(), "video")
	file, err := createPartFile(path, 1_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer file.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 1_000 {
		t.Fatalf("size = %d, want 1000", info.Size())
	}
}

// a part file left over from an interrupted run must not leak into the new one
func TestCreatePartFile_TruncatesStaleFile(t *testing.T) {
	path := partFilePath(t.TempDir(), "video")
	if err := os.WriteFile(path, []byte("stale data"), 0o644); err != nil {
		t.Fatal(err)
	}

	file, err := createPartFile(path, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	file.Close()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "\x00\x00\x00" {
		t.Fatalf("got %q, want 3 zero bytes", got)
	}
}

func TestPartFilePath_Hidden(t *testing.T) {
	if got, want := partFilePath("dir", "video"), filepath.Join("dir", ".video.part"); got != want {
		t.Fatalf("got %q, want %q", got, want)
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
