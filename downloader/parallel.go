package downloader

import "sync"

// runParallel calls task(0) ... task(n-1) concurrently, with at most maxParallel
// running at once (maxParallel <= 0 means no limit). It waits for all of them and
// returns the non-nil errors in the order the tasks finished.
func runParallel(n, maxParallel int, task func(i int) error) []error {
	if maxParallel <= 0 {
		maxParallel = n
	}
	var wg sync.WaitGroup
	wg.Add(n)
	errorsCh := make(chan error, n)
	semaphore := make(chan struct{}, maxParallel)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			if err := task(i); err != nil {
				errorsCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errorsCh)

	var errs []error
	for err := range errorsCh {
		errs = append(errs, err)
	}
	return errs
}
