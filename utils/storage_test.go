package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSections(t *testing.T, targetPath, resourceName string, contents []string) [][2]int {
	t.Helper()
	sections := make([][2]int, len(contents))
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
func TestMergeFiles_AppendsToExistingFile(t *testing.T) {
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
	if string(got) != "oldnew" {
		t.Fatalf("got %q, want %q", got, "oldnew")
	}
}

func TestMergeFiles_MissingSection(t *testing.T) {
	dir := t.TempDir()
	if err := mergeFiles(dir, "video", make([][2]int, 1)); err == nil {
		t.Fatal("expected error for missing section file")
	}
}

func TestRemoveTempFiles_IgnoresMissing(t *testing.T) {
	dir := t.TempDir()
	sections := writeSections(t, dir, "video", []string{"a"})
	sections = append(sections, [2]int{}) // section 1 was never written

	d := &Download{TargetPath: dir, ResourceName: "video"}
	if err := d.removeTempFiles(sections); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoTempFiles(t, dir)
}
