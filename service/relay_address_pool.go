package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Hhz0823/1s-ui/database/model"
)

// Bind the entire planned pool before any readiness or egress checks. The
// returned owned list includes successful additions even on partial failure,
// so the caller can roll back precisely the addresses it created.
func bindRelayAddressPool(ctx context.Context, items []model.RelayItem, existing map[string]bool, allowAdd bool,
	add func(context.Context, string, string, int) error, progress func(int, int)) ([]model.RelayItem, error) {
	jobs := make(chan int, len(items))
	for i, item := range items {
		if item.IPv6 == "" || existing[item.IPv6] {
			continue
		}
		if !allowAdd {
			return nil, fmt.Errorf("IPv6 %s is not currently assigned; enable system address creation", item.IPv6)
		}
		jobs <- i
	}
	close(jobs)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var owned []model.RelayItem
	var failure error
	total := len(jobs)
	for range min(4, total) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				item := &items[i]
				err := add(ctx, item.Interface, item.IPv6, item.Prefix)
				mu.Lock()
				if err != nil {
					if failure == nil {
						failure = err
						cancel()
					}
					mu.Unlock()
					return
				}
				item.AddedByUs = true
				owned = append(owned, *item)
				if progress != nil {
					progress(len(owned), total)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if failure != nil {
		return owned, failure
	}
	return owned, ctx.Err()
}

func waitRelayPoolSettle(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
