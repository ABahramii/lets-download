package downloader

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"time"
)

// sectionIdleTimeout is how long a section download may go without receiving
// any data before it is aborted. It is a variable so tests can shorten it.
var sectionIdleTimeout = 30 * time.Second

// errStalled is returned when a section receives no data for sectionIdleTimeout.
var errStalled = errors.New("server stopped sending data")

// idleWatchdog cancels a request when no data has been read for its timeout.
// Unlike an overall timeout, it never cuts off a slow but steady download.
type idleWatchdog struct {
	timeout time.Duration
	timer   *time.Timer
	expired atomic.Bool
}

// withIdleTimeout returns a context that is cancelled once the watchdog expires,
// and the watchdog itself. Call stop when the request is done.
func withIdleTimeout(parent context.Context, timeout time.Duration) (context.Context, *idleWatchdog) {
	ctx, cancel := context.WithCancel(parent)
	w := &idleWatchdog{timeout: timeout}
	w.timer = time.AfterFunc(timeout, func() {
		w.expired.Store(true)
		cancel()
	})
	return ctx, w
}

// reader wraps r so that every read that returns data restarts the timeout.
func (w *idleWatchdog) reader(r io.Reader) io.Reader {
	return readerFunc(func(p []byte) (int, error) {
		n, err := r.Read(p)
		if n > 0 {
			w.timer.Reset(w.timeout)
		}
		return n, err
	})
}

// err replaces the error caused by the watchdog cancelling the request with errStalled.
func (w *idleWatchdog) err(err error) error {
	if err != nil && w.expired.Load() {
		return errStalled
	}
	return err
}

func (w *idleWatchdog) stop() {
	w.timer.Stop()
}

type readerFunc func(p []byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }
