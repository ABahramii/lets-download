package downloader

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunParallel_RunsAllTasks(t *testing.T) {
	var ran [5]atomic.Bool
	errs := runParallel(len(ran), 0, func(i int) error {
		ran[i].Store(true)
		return nil
	})

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for i := range ran {
		if !ran[i].Load() {
			t.Fatalf("task %d did not run", i)
		}
	}
}

func TestRunParallel_CollectsErrors(t *testing.T) {
	errOdd := errors.New("odd")
	errs := runParallel(6, 2, func(i int) error {
		if i%2 == 1 {
			return errOdd
		}
		return nil
	})

	if len(errs) != 3 {
		t.Fatalf("got %d errors, want 3", len(errs))
	}
	for _, err := range errs {
		if !errors.Is(err, errOdd) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestRunParallel_RespectsLimit(t *testing.T) {
	const maxParallel = 3
	var inFlight, peak atomic.Int32

	runParallel(10, maxParallel, func(int) error {
		current := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			old := peak.Load()
			if current <= old || peak.CompareAndSwap(old, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		return nil
	})

	if got := peak.Load(); got > maxParallel {
		t.Fatalf("peak = %d, want <= %d", got, maxParallel)
	}
}

func TestRunParallel_Zero(t *testing.T) {
	if errs := runParallel(0, 0, func(int) error { return errors.New("never called") }); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}
