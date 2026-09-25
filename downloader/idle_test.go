package downloader

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func setSectionIdleTimeout(t *testing.T, timeout time.Duration) {
	t.Helper()
	old := sectionIdleTimeout
	sectionIdleTimeout = timeout
	t.Cleanup(func() { sectionIdleTimeout = old })
}

// newChunkedServer answers HEAD with the size and each range request with 206,
// then calls send to write the body.
func newChunkedServer(t *testing.T, size int, send func(w http.ResponseWriter, r *http.Request, length int)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(size))
			return
		}
		var start, end int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		length := end - start + 1
		w.Header().Set("Content-Length", strconv.Itoa(length))
		w.WriteHeader(http.StatusPartialContent)
		send(w, r, length)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestDownload_StalledSectionTimesOut(t *testing.T) {
	setSectionIdleTimeout(t, 200*time.Millisecond)
	// sends half of each section, then stops sending until the client gives up
	// (or the test ends, so a failing test doesn't block server.Close)
	release := make(chan struct{})
	server := newChunkedServer(t, 1_000, func(w http.ResponseWriter, r *http.Request, length int) {
		w.Write(make([]byte, length/2))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) }) // runs before server.Close
	targetPath := t.TempDir()

	done := make(chan error, 1)
	go func() { done <- newTestDownload(server.URL+"/file", targetPath, "file").Do() }()

	select {
	case err := <-done:
		if !errors.Is(err, errStalled) {
			t.Fatalf("expected errStalled, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Do did not return for a stalled server")
	}
	assertNoTempFiles(t, targetPath)
}

// a slow but steady server must not be cut off, even when the whole section
// takes much longer than the idle timeout
func TestDownload_SlowSteadySectionSucceeds(t *testing.T) {
	setSectionIdleTimeout(t, 200*time.Millisecond)
	const chunks = 10
	server := newChunkedServer(t, 1_000, func(w http.ResponseWriter, r *http.Request, length int) {
		for i := 0; i < chunks; i++ {
			time.Sleep(60 * time.Millisecond)
			w.Write(make([]byte, length/chunks))
			w.(http.Flusher).Flush()
		}
	})
	targetPath := t.TempDir()

	d := newTestDownload(server.URL+"/file", targetPath, "file")
	d.TotalSections = 1
	start := time.Now()
	if err := d.Do(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 2*sectionIdleTimeout {
		t.Fatalf("download took %v; the test needs it to take longer than the idle timeout", elapsed)
	}
	assertDownloaded(t, targetPath, "file", make([]byte, 1_000))
}
