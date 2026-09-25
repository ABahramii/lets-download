package utils

import (
	"bytes"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newFileServer serves the given files by name and supports HEAD and Range requests.
func newFileServer(t *testing.T, files map[string][]byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		content, ok := files[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
	}))
	t.Cleanup(server.Close)
	return server
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func newTestDownload(url, targetPath, resourceName string) *Download {
	return &Download{
		URL:           url,
		TargetPath:    targetPath,
		ResourceName:  resourceName,
		TotalSections: 10,
	}
}

func assertDownloaded(t *testing.T, targetPath, resourceName string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(targetPath, resourceName+".mp4"))
	if err != nil {
		t.Fatalf("reading %s: %v", resourceName, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s: content mismatch (got %d bytes, want %d bytes)", resourceName, len(got), len(want))
	}
}

func assertNoTempFiles(t *testing.T, targetPath string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(targetPath, "*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temp files left behind: %v", matches)
	}
}

func TestDownloadAll(t *testing.T) {
	files := map[string][]byte{
		"file1": randomBytes(t, 10_000),
		"file2": randomBytes(t, 25_003),
		"file3": randomBytes(t, 4_321),
	}
	server := newFileServer(t, files)
	targetPath := t.TempDir()

	var downloads []*Download
	for name := range files {
		downloads = append(downloads, newTestDownload(server.URL+"/"+name, targetPath, name))
	}

	if err := DownloadAll(downloads); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for name, content := range files {
		assertDownloaded(t, targetPath, name, content)
	}
	assertNoTempFiles(t, targetPath)
}

func TestDownloadAll_PartialFailure(t *testing.T) {
	content := randomBytes(t, 8_000)
	server := newFileServer(t, map[string][]byte{"good": content})
	targetPath := t.TempDir()

	badURL := server.URL + "/missing"
	downloads := []*Download{
		newTestDownload(server.URL+"/good", targetPath, "good"),
		newTestDownload(badURL, targetPath, "missing"),
	}

	err := DownloadAll(downloads)
	if err == nil {
		t.Fatal("expected error for missing resource")
	}
	if !strings.Contains(err.Error(), badURL) {
		t.Fatalf("error should mention %s, got: %v", badURL, err)
	}

	assertDownloaded(t, targetPath, "good", content)
}
