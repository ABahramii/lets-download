package downloader

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
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
		"downloaded 100 bytes from section 9: [900 999]\n",
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

func TestDownload_SectionErrorMessage(t *testing.T) {
	content := randomBytes(t, 1_000)
	// fails only section 3 (bytes 300-399 of 10 sections)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "bytes=300-399" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.ServeContent(w, r, "file", time.Time{}, bytes.NewReader(content))
	}))
	t.Cleanup(server.Close)
	targetPath := t.TempDir()

	err := newTestDownload(server.URL+"/file", targetPath, "file").Do()

	want := "failed to download section 3: unexpected response code 503 for range request"
	if err == nil || err.Error() != want {
		t.Fatalf("got error %v, want %q", err, want)
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
	tests := []struct {
		name       string
		firstName  string
		secondName string
	}{
		{name: "same name", firstName: "video", secondName: "video"},
		// the same file on case-insensitive disks (macOS, Windows)
		{name: "names differ only in case", firstName: "Video", secondName: "video"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := randomBytes(t, 6_000)
			second := randomBytes(t, 7_000)
			server := newFileServer(t, map[string][]byte{"a/" + tt.firstName: first, "b/" + tt.secondName: second})
			targetPath := t.TempDir()

			duplicateURL := server.URL + "/b/" + tt.secondName
			downloads := []*Download{
				newTestDownload(server.URL+"/a/"+tt.firstName, targetPath, tt.firstName),
				newTestDownload(duplicateURL, targetPath, tt.secondName),
			}

			err := DownloadAll(downloads, 2)
			if !errors.Is(err, ErrDuplicateOutput) {
				t.Fatalf("expected ErrDuplicateOutput, got: %v", err)
			}
			if !strings.Contains(err.Error(), duplicateURL) {
				t.Fatalf("error should mention %s, got: %v", duplicateURL, err)
			}

			// the first download in the list wins and is not corrupted by the duplicate
			assertDownloaded(t, targetPath, tt.firstName, first)
			assertNoTempFiles(t, targetPath)
		})
	}
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

// files smaller than the section count, and empty files, must download correctly
func TestDownload_SmallFiles(t *testing.T) {
	for _, size := range []int{0, 1, 5, 9, 10, 11} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			content := randomBytes(t, size)
			server := newFileServer(t, map[string][]byte{"file": content})
			targetPath := t.TempDir()

			if err := newTestDownload(server.URL+"/file", targetPath, "file").Do(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertDownloaded(t, targetPath, "file", content)
			assertNoTempFiles(t, targetPath)
		})
	}
}

// Download.TotalSections controls how many range requests are made
func TestDownload_UsesTotalSections(t *testing.T) {
	for _, totalSections := range []int{0, 1, 3, 25} {
		t.Run(strconv.Itoa(totalSections), func(t *testing.T) {
			content := randomBytes(t, 1_000)
			var rangeRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Range") != "" {
					rangeRequests.Add(1)
				}
				http.ServeContent(w, r, "file", time.Time{}, bytes.NewReader(content))
			}))
			t.Cleanup(server.Close)
			targetPath := t.TempDir()

			d := newTestDownload(server.URL+"/file", targetPath, "file")
			d.TotalSections = totalSections
			if err := d.Do(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			want := int32(totalSections)
			if totalSections <= 0 {
				want = defaultSectionCount
			}
			if got := rangeRequests.Load(); got != want {
				t.Fatalf("got %d range requests, want %d", got, want)
			}
			assertDownloaded(t, targetPath, "file", content)
			assertNoTempFiles(t, targetPath)
		})
	}
}

// newRangeServer answers HEAD with size and hands every range request to respond,
// which writes the whole response.
func newRangeServer(t *testing.T, size int, respond func(w http.ResponseWriter, section byteRange)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(size))
			return
		}
		var section byteRange
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &section.start, &section.end); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respond(w, section)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestDownload_WrongSectionResponse(t *testing.T) {
	tests := []struct {
		name    string
		respond func(w http.ResponseWriter, section byteRange)
		wantErr string
	}{
		{
			// a server that caps chunks at 50 bytes and says so in Content-Range
			name: "different Content-Range",
			respond: func(w http.ResponseWriter, section byteRange) {
				end := min(section.end, section.start+49)
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/100", section.start, end))
				w.Header().Set("Content-Length", strconv.Itoa(end-section.start+1))
				w.WriteHeader(http.StatusPartialContent)
				w.Write(make([]byte, end-section.start+1))
			},
			wantErr: "server sent bytes 0-49, requested 0-99",
		},
		{
			name: "invalid Content-Range",
			respond: func(w http.ResponseWriter, section byteRange) {
				w.Header().Set("Content-Range", "bytes garbage")
				w.WriteHeader(http.StatusPartialContent)
			},
			wantErr: `invalid Content-Range "bytes garbage"`,
		},
		{
			// no Content-Range and no Content-Length, so only the byte count can catch it
			name: "short body",
			respond: func(w http.ResponseWriter, section byteRange) {
				w.WriteHeader(http.StatusPartialContent)
				w.Write(make([]byte, section.length()/2))
			},
			wantErr: "received 50 bytes for range 0-99, want 100",
		},
		{
			name: "long body",
			respond: func(w http.ResponseWriter, section byteRange) {
				w.WriteHeader(http.StatusPartialContent)
				w.Write(make([]byte, section.length()+10))
			},
			wantErr: "received more than 100 bytes for range 0-99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// one 100-byte section, so the failing section and its range are known
			server := newRangeServer(t, 100, tt.respond)
			targetPath := t.TempDir()

			d := newTestDownload(server.URL+"/file", targetPath, "file")
			d.TotalSections = 1
			err := d.Do()

			want := "failed to download section 0: " + tt.wantErr
			if err == nil || err.Error() != want {
				t.Fatalf("got error %v, want %q", err, want)
			}
			assertNoTempFiles(t, targetPath)
			if _, statErr := os.Stat(filepath.Join(targetPath, "file.mp4")); !os.IsNotExist(statErr) {
				t.Fatalf("output file must not be created, stat error: %v", statErr)
			}
		})
	}
}

// fakeProgress records what a Download reports to its Progress.
type fakeProgress struct {
	mu       sync.Mutex
	sizes    []int64
	added    []int64 // bytes per section
	starts   int
	finishes int
	err      error
}

func (p *fakeProgress) Start(sectionSizes []int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sizes = sectionSizes
	p.added = make([]int64, len(sectionSizes))
	p.starts++
}

func (p *fakeProgress) Add(section, n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.added[section] += int64(n)
}

func (p *fakeProgress) Finish(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err = err
	p.finishes++
}

func TestDownload_ReportsProgress(t *testing.T) {
	content := randomBytes(t, 5_000)
	server := newFileServer(t, map[string][]byte{"file": content})
	progress := &fakeProgress{}
	d := newTestDownload(server.URL+"/file", t.TempDir(), "file")
	d.Progress = progress

	if err := d.Do(); err != nil {
		t.Fatal(err)
	}
	if progress.starts != 1 || len(progress.sizes) != d.TotalSections {
		t.Fatalf("Start called %d times with %d sections, want once with %d", progress.starts, len(progress.sizes), d.TotalSections)
	}
	var total int64
	for i, size := range progress.sizes {
		total += size
		if progress.added[i] != size {
			t.Errorf("section %d: Add reported %d bytes, want %d", i, progress.added[i], size)
		}
	}
	if total != int64(len(content)) {
		t.Fatalf("section sizes add up to %d, want %d", total, len(content))
	}
	if progress.finishes != 1 || progress.err != nil {
		t.Fatalf("Finish called %d times with %v, want once with nil", progress.finishes, progress.err)
	}
}

func TestDownload_ReportsProgressError(t *testing.T) {
	// ignores Range and always sends the full body with 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5000")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	progress := &fakeProgress{}
	d := newTestDownload(server.URL+"/file", t.TempDir(), "file")
	d.Progress = progress

	err := d.Do()
	if err == nil {
		t.Fatal("expected error when server ignores Range")
	}
	if progress.finishes != 1 || progress.err != err {
		t.Fatalf("Finish called %d times with %v, want once with %v", progress.finishes, progress.err, err)
	}
}

func TestDownload_ReportsProgressErrorBeforeStart(t *testing.T) {
	server := newFileServer(t, nil)
	progress := &fakeProgress{}
	d := newTestDownload(server.URL+"/missing", t.TempDir(), "missing")
	d.Progress = progress

	if err := d.Do(); err == nil {
		t.Fatal("expected error for missing file")
	}
	if progress.starts != 0 || progress.finishes != 1 || progress.err == nil {
		t.Fatalf("got %d starts and %d finishes with %v, want 0 starts and 1 finish with an error", progress.starts, progress.finishes, progress.err)
	}
}
