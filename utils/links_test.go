package utils

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeLinksFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "links.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadLinks(t *testing.T) {
	path := writeLinksFile(t, "http://a.com/file1\n\n  http://b.com/file2  \n# comment\nhttp://c.com/file3?filename=x.zip\n")

	links, err := ReadLinks(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"http://a.com/file1", "http://b.com/file2", "http://c.com/file3?filename=x.zip"}
	if !reflect.DeepEqual(links, want) {
		t.Fatalf("got %v, want %v", links, want)
	}
}

func TestReadLinks_MissingFile(t *testing.T) {
	_, err := ReadLinks(filepath.Join(t.TempDir(), "not-exists.txt"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadLinks_NoLinks(t *testing.T) {
	path := writeLinksFile(t, "\n   \n# only a comment\n")

	_, err := ReadLinks(path)
	if err == nil {
		t.Fatal("expected error for file without links")
	}
}
