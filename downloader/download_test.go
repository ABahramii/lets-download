package downloader

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
		Out:           io.Discard,
	}
}

// syncBuffer is a bytes.Buffer that is safe for concurrent writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestDownload_WritesProgressToOut(t *testing.T) {
	content := randomBytes(t, 1_000)
	server := newFileServer(t, map[string][]byte{"file": content})
	var out syncBuffer

	d := newTestDownload(server.URL+"/file", t.TempDir(), "file")
	d.Out = &out
	if err := d.Do(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"making connection\n",
		"status: 200\n",
		"file: file\nsize: 1000 bytes\n",
		"downloaded 100 bytes from section 0: [0 99]\n",
		"downloaded 100 bytes from section 9: [900 1000]\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
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

func TestNewDownload(t *testing.T) {
	d, err := NewDownload("https://a.com/dir/file.zip", "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.URL != "https://a.com/dir/file.zip" || d.TargetPath != "/tmp" || d.ResourceName != "file.zip" || d.TotalSections != 10 {
		t.Fatalf("unexpected download: %+v", d)
	}
}

func TestNewDownload_Invalid(t *testing.T) {
	tests := []struct {
		url     string
		wantErr string
	}{
		{url: "foo bar", wantErr: "URL is invalid"},
		{url: "ftp://a.com/file", wantErr: "URL is invalid"},
		{url: "http:///file", wantErr: "URL is invalid"},
		{url: "http://a.com/%zz", wantErr: "URL is invalid"},
		{url: "http://a.com/", wantErr: "no resource name found in URL"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			_, err := NewDownload(tt.url, "/tmp")
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("got %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestDownloadAll_DuplicateResourceName(t *testing.T) {
	first := randomBytes(t, 6_000)
	second := randomBytes(t, 7_000)
	server := newFileServer(t, map[string][]byte{"a/video": first, "b/video": second})
	targetPath := t.TempDir()

	duplicateURL := server.URL + "/b/video"
	downloads := []*Download{
		newTestDownload(server.URL+"/a/video", targetPath, "video"),
		newTestDownload(duplicateURL, targetPath, "video"),
	}

	err := DownloadAll(downloads, 2)
	if !errors.Is(err, ErrDuplicateOutput) {
		t.Fatalf("expected ErrDuplicateOutput, got: %v", err)
	}
	if !strings.Contains(err.Error(), duplicateURL) {
		t.Fatalf("error should mention %s, got: %v", duplicateURL, err)
	}

	// the first download in the list wins and is not corrupted by the duplicate
	assertDownloaded(t, targetPath, "video", first)
	assertNoTempFiles(t, targetPath)
}

func TestDownloadAll_SameNameDifferentTargetPaths(t *testing.T) {
	first := randomBytes(t, 6_000)
	second := randomBytes(t, 7_000)
	server := newFileServer(t, map[string][]byte{"a/video": first, "b/video": second})
	firstPath, secondPath := t.TempDir(), t.TempDir()

	downloads := []*Download{
		newTestDownload(server.URL+"/a/video", firstPath, "video"),
		newTestDownload(server.URL+"/b/video", secondPath, "video"),
	}

	if err := DownloadAll(downloads, 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertDownloaded(t, firstPath, "video", first)
	assertDownloaded(t, secondPath, "video", second)
}

// a re-run must leave exactly one copy of the file, not two appended together
func TestDownload_RerunReplacesOutput(t *testing.T) {
	content := randomBytes(t, 5_000)
	server := newFileServer(t, map[string][]byte{"file": content})
	targetPath := t.TempDir()

	for run := 1; run <= 2; run++ {
		if err := newTestDownload(server.URL+"/file", targetPath, "file").Do(); err != nil {
			t.Fatalf("run %d: unexpected error: %v", run, err)
		}
	}

	assertDownloaded(t, targetPath, "file", content)
	assertNoTempFiles(t, targetPath)
}
