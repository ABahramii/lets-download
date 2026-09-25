package downloader

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSections(t *testing.T, targetPath, resourceName string, contents []string) []byteRange {
	t.Helper()
	sections := make([]byteRange, len(contents))
	for i, content := range contents {
		if err := os.WriteFile(sectionFilePath(targetPath, resourceName, i), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return sections
}

func TestMergeFiles(t *testing.T) {
	dir := t.TempDir()
	sections := writeSections(t, dir, "video", []string{"aaa", "bbb", "ccc"})

	if err := mergeFiles(dir, "video", sections); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "video.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "aaabbbccc" {
		t.Fatalf("got %q, want %q", got, "aaabbbccc")
	}
}

// current behavior: an existing output file is appended to, not replaced
// a re-run must replace the output file, not append to it
func TestMergeFiles_ReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "video.mp4"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	sections := writeSections(t, dir, "video", []string{"new"})

	if err := mergeFiles(dir, "video", sections); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "video.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("got %q, want %q", got, "new")
	}
	assertNoPartFiles(t, dir)
}

func TestMergeFiles_FailureKeepsExistingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "video.mp4"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// section 0 exists, section 1 is missing, so the merge fails partway
	sections := append(writeSections(t, dir, "video", []string{"new"}), byteRange{})

	if err := mergeFiles(dir, "video", sections); err == nil {
		t.Fatal("expected error for missing section file")
	}

	got, err := os.ReadFile(filepath.Join(dir, "video.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("existing output changed: got %q, want %q", got, "old")
	}
	assertNoPartFiles(t, dir)
}

func assertNoPartFiles(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.part"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("partial files left behind: %v", matches)
	}
}

func TestMergeFiles_MissingSection(t *testing.T) {
	dir := t.TempDir()
	if err := mergeFiles(dir, "video", make([]byteRange, 1)); err == nil {
		t.Fatal("expected error for missing section file")
	}
}

func TestRemoveTempFiles_IgnoresMissing(t *testing.T) {
	dir := t.TempDir()
	sections := writeSections(t, dir, "video", []string{"a"})
	sections = append(sections, byteRange{}) // section 1 was never written

	d := &Download{TargetPath: dir, ResourceName: "video"}
	if err := d.removeTempFiles(sections); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoTempFiles(t, dir)
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
