package utils

import (
	"bytes"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
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

	if err := DownloadAll(downloads, 2); err != nil {
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

	err := DownloadAll(downloads, 2)
	if err == nil {
		t.Fatal("expected error for missing resource")
	}
	if !strings.Contains(err.Error(), badURL) {
		t.Fatalf("error should mention %s, got: %v", badURL, err)
	}

	assertDownloaded(t, targetPath, "good", content)
	assertNoTempFiles(t, targetPath)
}

func TestDownloadAll_Empty(t *testing.T) {
	if err := DownloadAll(nil, 4); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownloadAll_MaxParallel(t *testing.T) {
	const maxParallel = 2
	content := randomBytes(t, 2_000)
	var inFlight, peak atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// every download starts with one HEAD request, so in-flight HEADs = running downloads
		if r.Method == http.MethodHead {
			current := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				old := peak.Load()
				if current <= old || peak.CompareAndSwap(old, current) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		http.ServeContent(w, r, "file", time.Time{}, bytes.NewReader(content))
	}))
	t.Cleanup(server.Close)
	targetPath := t.TempDir()

	var downloads []*Download
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		downloads = append(downloads, newTestDownload(server.URL+"/"+name, targetPath, name))
	}

	if err := DownloadAll(downloads, maxParallel); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := peak.Load(); got > maxParallel {
		t.Fatalf("peak parallel downloads = %d, want <= %d", got, maxParallel)
	}
}

func TestDownload_NonPartialResponse(t *testing.T) {
	content := randomBytes(t, 5_000)
	// ignores Range and always sends the full body with 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			w.Write(content)
		}
	}))
	t.Cleanup(server.Close)
	targetPath := t.TempDir()

	err := newTestDownload(server.URL+"/file", targetPath, "file").Do()
	if err == nil {
		t.Fatal("expected error when server ignores Range")
	}
	assertNoTempFiles(t, targetPath)
}
