package tray

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestConcurrentRefreshIsSerialized is finding #22: the poll loop and a menu
// click both call refresh, which reads and writes the signature and the
// snapshot. The whole refresh must be serialized (run with -race).
func TestConcurrentRefreshIsSerialized(t *testing.T) {
	const workers = 4
	arrived := make(chan struct{}, workers)
	release := make(chan struct{})
	o := Options{
		NoNotify: true,
		Status: func(context.Context) (Status, error) {
			arrived <- struct{}{}
			<-release
			return Status{Ready: true, Running: 1, AccountStatus: "connected"}, nil
		},
		Jobs: func(context.Context) ([]Job, error) {
			return []Job{{SourceName: "Alpha", State: "running", Completed: 1}}, nil
		},
		Copy: func(context.Context, string) (string, error) { return "", nil },
		Now:  func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
	a := newAgent(o)
	a.p = &fakePresenter{}

	// Release every refresh once all of them reached the daemon call, or after
	// a deadline when a serialized refresh never lets more than one in.
	go func() {
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		for range workers {
			select {
			case <-arrived:
			case <-timer.C:
				close(release)
				return
			}
		}
		close(release)
	}()

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.refresh(context.Background())
		}()
	}
	wg.Wait()
}
