package service

import (
	"context"
	"fmt"
	"net/netip"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Hhz0823/1s-ui/database/model"
)

type relayCreationChecks struct {
	skipEgress bool
	add        func(context.Context, string, string, int) error
	ready      func(context.Context, []model.RelayItem) ([]error, error)
	ipv6       relayIPv6EgressProbe
	ipv4       func(context.Context, RelayUpstream) error
	settle     func(context.Context) error
	progress   func(string, int, int)
}

// Check all rows independently. A stage deadline skips unfinished rows, while
// cancellation of the HTTP request aborts the whole operation before saving.
func checkRelayRows(parent context.Context, count, workers int, timeout time.Duration,
	check func(context.Context, int) error, progress func(int, int)) ([]error, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	results := make([]error, count)
	jobs := make(chan int, count)
	for i := range results {
		results[i] = fmt.Errorf("stage time limit reached before this row passed")
		jobs <- i
	}
	close(jobs)
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0
	for range min(workers, count) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				err := check(ctx, i)
				mu.Lock()
				results[i] = err
				completed++
				if progress != nil {
					progress(completed, count)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return results, parent.Err()
}

func relayItemUpstream(item model.RelayItem) RelayUpstream {
	return RelayUpstream{Server: item.UpstreamServer, Port: item.UpstreamPort,
		Username: item.UpstreamUsername, Password: item.UpstreamPassword}
}

// planned remains the ownership ledger, including rows dropped by later checks.
// The caller must clean up added addresses on failure and skipped rows on success.
func prepareUsableRelayItems(ctx context.Context, planned []model.RelayItem, existing map[string]bool,
	allowAdd bool, checks relayCreationChecks) ([]model.RelayItem, *model.RelayCreationReport, error) {
	report := &model.RelayCreationReport{Requested: len(planned), Skipped: []model.RelaySkippedItem{}}
	for i := range planned {
		planned[i].SourceRow = i + 1
	}
	items := planned
	progress := func(stage string) func(int, int) {
		if checks.progress != nil {
			checks.progress(stage, 0, len(items))
		}
		return func(done, total int) {
			if checks.progress != nil {
				checks.progress(stage, done, total)
			}
		}
	}
	filter := func(stage string, failures []error) {
		kept := make([]model.RelayItem, 0, len(items))
		for i, item := range items {
			if failures[i] == nil {
				kept = append(kept, item)
				continue
			}
			reason := failures[i].Error()
			for _, secret := range []string{item.UpstreamPassword, item.UpstreamUsername} {
				if secret != "" {
					reason = strings.ReplaceAll(reason, secret, "[redacted]")
				}
			}
			report.Skipped = append(report.Skipped, model.RelaySkippedItem{
				Row: item.SourceRow, IPv6: item.IPv6, Stage: stage, Reason: reason,
			})
		}
		items = kept
	}
	if checks.ipv6 != nil {
		failures, err := checkRelayRows(ctx, len(items), 4, 30*time.Second, func(ctx context.Context, i int) error {
			item := &planned[i]
			if existing[item.IPv6] {
				return nil
			}
			if !allowAdd {
				return fmt.Errorf("IPv6 is not assigned to the interface; enable system address creation")
			}
			if err := checks.add(ctx, item.Interface, item.IPv6, item.Prefix); err != nil {
				return err
			}
			item.AddedByUs = true
			return nil
		}, progress("addresses"))
		if err != nil {
			return nil, report, err
		}
		filter("addresses", failures)
		if len(items) > 0 {
			update := progress("dad")
			failures, err = checks.ready(ctx, items)
			if err != nil {
				return nil, report, err
			}
			update(len(items), len(items))
			filter("dad", failures)
		}
		if len(items) > 0 && !checks.skipEgress {
			for _, item := range items {
				if item.AddedByUs && checks.settle != nil {
					progress("settling")
					if err := checks.settle(ctx); err != nil {
						return nil, report, err
					}
					break
				}
			}
			failures, err = checkRelayRows(ctx, len(items), relayIPv6ProbeWorkers, 45*time.Second, func(ctx context.Context, i int) error {
				address, err := netip.ParseAddr(items[i].IPv6)
				if err != nil || !address.Is6() {
					return fmt.Errorf("invalid IPv6 address")
				}
				return checks.ipv6(ctx, address)
			}, progress("ipv6"))
			if err != nil {
				return nil, report, err
			}
			filter("ipv6", failures)
		}
	}
	if checks.ipv4 != nil && !checks.skipEgress && len(items) > 0 {
		failures, err := checkRelayRows(ctx, len(items), 8, 45*time.Second, func(ctx context.Context, i int) error {
			return checks.ipv4(ctx, relayItemUpstream(items[i]))
		}, progress("ipv4"))
		if err != nil {
			return nil, report, err
		}
		filter("ipv4", failures)
	}
	sort.Slice(report.Skipped, func(i, j int) bool { return report.Skipped[i].Row < report.Skipped[j].Row })
	if err := ctx.Err(); err != nil {
		return nil, report, err
	}
	if len(items) == 0 {
		return nil, report, fmt.Errorf("no usable relay rows (0/%d passed); no nodes created", len(planned))
	}
	if checks.ipv6 != nil {
		for i := range items {
			items[i].EgressCheck = "passed"
			if checks.skipEgress {
				items[i].EgressCheck = "not_checked"
			}
		}
	}
	return items, report, nil
}

// Read the interface once per polling cycle, so one tentative/duplicate address
// neither rejects ready neighbours nor causes hundreds of individual ip scans.
func checkRelayRowsReady(parent context.Context, items []model.RelayItem) ([]error, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	results := make([]error, len(items))
	pending := make(map[string][]int)
	for i, item := range items {
		results[i] = fmt.Errorf("IPv6 address did not become ready before timeout")
		pending[item.Interface] = append(pending[item.Interface], i)
	}
	for len(pending) > 0 && ctx.Err() == nil {
		for iface, indices := range pending {
			attempt, stop := context.WithTimeout(ctx, 2*time.Second)
			output, err := exec.CommandContext(attempt, "ip", "-6", "-o", "addr", "show", "dev", iface).CombinedOutput()
			stop()
			if err != nil {
				if ctx.Err() != nil {
					break
				}
				return nil, fmt.Errorf("read IPv6 interface state: %w", err)
			}
			waiting := make([]int, 0, len(indices))
			for _, i := range indices {
				switch relayIPv6AddressState(string(output), items[i].IPv6) {
				case relayAddressReady:
					results[i] = nil
				case relayAddressDADFailed:
					results[i] = fmt.Errorf("IPv6 duplicate-address detection failed")
				default:
					waiting = append(waiting, i)
				}
			}
			if len(waiting) == 0 {
				delete(pending, iface)
			} else {
				pending[iface] = waiting
			}
		}
		if len(pending) > 0 {
			_ = waitRelayPoolSettle(ctx, 200*time.Millisecond)
		}
	}
	return results, parent.Err()
}

func cleanupSkippedRelayAddresses(planned, kept []model.RelayItem, remove func(string, string, int) error) error {
	retained := make(map[string]bool, len(kept))
	for _, item := range kept {
		retained[item.IPv6] = true
	}
	for i := range planned {
		item := &planned[i]
		if item.AddedByUs && !retained[item.IPv6] {
			if err := remove(item.Interface, item.IPv6, item.Prefix); err != nil {
				return fmt.Errorf("cleanup skipped row %d: %w", item.SourceRow, err)
			}
			item.AddedByUs = false
		}
	}
	return nil
}
