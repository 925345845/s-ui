package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Hhz0823/1s-ui/database/model"
)

func TestRelayMatchingUsableIPv6DoesNotDiscardItsIPv4(t *testing.T) {
	planned, checks := relayPartialFixture(6)
	checks.matchUsable = true
	checks.ipv6 = func(_ context.Context, ip netip.Addr) error {
		if ip.String() == planned[1].IPv6 || ip.String() == planned[4].IPv6 {
			return nil
		}
		return errors.New("unreachable")
	}
	checks.ipv4 = func(_ context.Context, u RelayUpstream) error {
		if u.Username == "user-2" {
			return errors.New("user-2 password-2 rejected")
		}
		return nil
	}
	kept, report, err := prepareUsableRelayItems(context.Background(), planned, nil, true, checks)
	if err != nil || len(kept) != 2 {
		t.Fatalf("kept=%+v report=%+v err=%v", kept, report, err)
	}
	for i, row := range []int{1, 3} {
		candidate := []int{2, 5}[i]
		if kept[i].SourceRow != row || kept[i].IPv6SourceRow != candidate || kept[i].IPv6 != planned[candidate-1].IPv6 || relayItemUpstream(kept[i]) != relayItemUpstream(planned[row-1]) {
			t.Fatalf("bad match: %+v", kept[i])
		}
	}
	if !reflect.DeepEqual(report.UnmatchedIPv4, []int{2, 4, 5, 6}) {
		t.Fatalf("missing unmatched rows: %+v", report)
	}
	for _, failure := range report.Skipped {
		if strings.Contains(failure.Reason, "password-2") || strings.Contains(failure.Reason, "user-2") {
			t.Fatal("secret leaked")
		}
	}
	var removed []string
	if err := cleanupSkippedRelayAddresses(planned, kept, func(_, ip string, _ int) error { removed = append(removed, ip); return nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(removed, []string{planned[0].IPv6, planned[2].IPv6, planned[3].IPv6, planned[5].IPv6}) {
		t.Fatalf("bad cleanup: %v", removed)
	}
}

func TestRelayMatchingAllIPv4FailDoesNotSaveIPv6OnlyPairs(t *testing.T) {
	planned, checks := relayPartialFixture(4)
	checks.matchUsable = true
	checks.ipv4 = func(context.Context, RelayUpstream) error { return errors.New("unreachable") }
	kept, report, err := prepareUsableRelayItems(context.Background(), planned, nil, true, checks)
	if err == nil || len(kept) != 0 || len(report.UnmatchedIPv4) != 4 {
		t.Fatalf("invalid success: %+v %v", report, err)
	}
}

func TestRelayMatchingLastCandidateStillPairsFirstIPv4(t *testing.T) {
	planned, checks := relayPartialFixture(500)
	checks.matchUsable = true
	var calls atomic.Int32
	checks.ipv6 = func(_ context.Context, ip netip.Addr) error {
		calls.Add(1)
		if ip.String() == planned[499].IPv6 {
			return nil
		}
		return errors.New("unreachable")
	}
	kept, report, err := prepareUsableRelayItems(context.Background(), planned, nil, true, checks)
	if err != nil || calls.Load() != 500 || len(kept) != 1 || kept[0].SourceRow != 1 || kept[0].IPv6SourceRow != 500 || len(report.UnmatchedIPv4) != 499 {
		t.Fatalf("kept=%+v probes=%d err=%v", kept, calls.Load(), err)
	}
}

func TestRelayChecksDefaultEnabledAndBudgetCoversEveryWave(t *testing.T) {
	for _, raw := range []string{`{}`, `{"verify_egress":true}`, `{"verify_egress":false}`} {
		var req RelayCreateRequest
		if err := json.Unmarshal([]byte(raw), &req); err != nil {
			t.Fatal(err)
		}
		skip := req.VerifyEgress != nil && !*req.VerifyEgress
		if skip != strings.Contains(raw, "false") {
			t.Fatalf("unexpected default for %s", raw)
		}
	}
	for _, count := range []int{1, 16, 17, 100, 500} {
		v6 := relayCheckBudget(count, 16, 36*time.Second)
		v4 := relayCheckBudget(count, 8, 12*time.Second)
		if v6 < time.Duration((count+15)/16)*36*time.Second || v4 < time.Duration((count+7)/8)*12*time.Second {
			t.Fatal("budget truncates queued rows")
		}
		if v6+v4+2*time.Minute >= 45*time.Minute {
			t.Fatal("remote deadline too short")
		}
	}
}

func relayPartialFixture(count int) ([]model.RelayItem, relayCreationChecks) {
	items := make([]model.RelayItem, count)
	for i := range items {
		items[i] = model.RelayItem{IPv6: fmt.Sprintf("2001:db8::%x", i+1), Interface: "eth0", Prefix: 64,
			UpstreamServer: fmt.Sprintf("proxy-%d.test", i+1), UpstreamPort: 1080 + i,
			UpstreamUsername: fmt.Sprintf("user-%d", i+1), UpstreamPassword: fmt.Sprintf("password-%d", i+1)}
	}
	return items, relayCreationChecks{
		add: func(context.Context, string, string, int) error { return nil },
		ready: func(_ context.Context, items []model.RelayItem) ([]error, error) {
			return make([]error, len(items)), nil
		},
		ipv6: func(context.Context, netip.Addr) error { return nil },
		ipv4: func(context.Context, RelayUpstream) error { return nil },
	}
}

func TestRelayPartialPreservesOriginalPairsAcrossEveryFailureStage(t *testing.T) {
	planned, checks := relayPartialFixture(7)
	original := append([]model.RelayItem(nil), planned...)
	checks.add = func(_ context.Context, _, ip string, _ int) error {
		if ip == planned[0].IPv6 {
			return errors.New("add failed")
		}
		return nil
	}
	checks.ready = func(_ context.Context, items []model.RelayItem) ([]error, error) {
		results := make([]error, len(items))
		for i, item := range items {
			if item.SourceRow == 3 {
				results[i] = errors.New("dadfailed")
			}
		}
		return results, nil
	}
	checks.ipv6 = func(_ context.Context, ip netip.Addr) error {
		if ip.String() == planned[4].IPv6 {
			return errors.New("IPv6 timeout")
		}
		return nil
	}
	checks.ipv4 = func(_ context.Context, upstream RelayUpstream) error {
		if upstream.Server == "proxy-6.test" {
			return errors.New("password-6 user-6 rejected")
		}
		return nil
	}
	// Failed row 5 was imported and must never be removed from the interface.
	kept, report, err := prepareUsableRelayItems(context.Background(), planned, map[string]bool{planned[4].IPv6: true}, true, checks)
	if err != nil || len(kept) != 3 || report.Requested != 7 || len(report.Skipped) != 4 {
		t.Fatalf("kept=%+v report=%+v err=%v", kept, report, err)
	}
	for i, row := range []int{2, 4, 7} {
		if kept[i].SourceRow != row || kept[i].IPv6 != original[row-1].IPv6 || relayItemUpstream(kept[i]) != relayItemUpstream(original[row-1]) {
			t.Fatalf("pair shifted: %+v", kept[i])
		}
	}
	for i, stage := range []string{"addresses", "dad", "ipv6", "ipv4"} {
		if report.Skipped[i].Stage != stage {
			t.Fatalf("incorrect report: %+v", report)
		}
	}
	if strings.Contains(report.Skipped[3].Reason, "password-6") || strings.Contains(report.Skipped[3].Reason, "user-6") {
		t.Fatal("credentials leaked")
	}
	var removed []string
	if err := cleanupSkippedRelayAddresses(planned, kept, func(_, ip string, _ int) error { removed = append(removed, ip); return nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(removed, []string{original[2].IPv6, original[5].IPv6}) {
		t.Fatalf("wrong cleanup: %v", removed)
	}
	// A later database/core failure must still be able to remove ALL kept additions.
	var rollback []int
	for _, item := range planned {
		if item.AddedByUs {
			rollback = append(rollback, item.SourceRow)
		}
	}
	if !reflect.DeepEqual(rollback, []int{2, 4, 7}) {
		t.Fatalf("lost ownership ledger: %v", rollback)
	}
}

func TestRelayPartialAllFailedAndAllPassed(t *testing.T) {
	for _, fail := range []bool{false, true} {
		planned, checks := relayPartialFixture(10)
		checks.ipv6 = func(context.Context, netip.Addr) error {
			if fail {
				return errors.New("unreachable")
			}
			return nil
		}
		kept, report, err := prepareUsableRelayItems(context.Background(), planned, nil, true, checks)
		if fail {
			if err == nil || len(kept) != 0 || len(report.Skipped) != 10 {
				t.Fatalf("all failed became success: %+v %v", report, err)
			}
		} else if err != nil || len(kept) != 10 || len(report.Skipped) != 0 {
			t.Fatalf("all passed: %+v %v", report, err)
		}
	}
}

func TestRelayPartialCancellationRetainsCleanupLedger(t *testing.T) {
	planned, checks := relayPartialFixture(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checks.ipv6 = func(context.Context, netip.Addr) error { cancel(); return nil }
	kept, _, err := prepareUsableRelayItems(ctx, planned, nil, true, checks)
	if !errors.Is(err, context.Canceled) || len(kept) != 0 {
		t.Fatalf("cancellation saved nodes: %v", err)
	}
	for _, item := range planned {
		if !item.AddedByUs {
			t.Fatal("lost address ownership on cancellation")
		}
	}
}

func TestRelayPartialDeadlineKeepsCompletedRowsAndSkipsQueued(t *testing.T) {
	errs, err := checkRelayRows(context.Background(), 5, 1, 30*time.Millisecond, func(ctx context.Context, i int) error {
		if i == 0 {
			return nil
		}
		<-ctx.Done()
		return ctx.Err()
	}, nil)
	if err != nil || errs[0] != nil {
		t.Fatalf("completed row lost: %v %v", errs, err)
	}
	for _, e := range errs[1:] {
		if e == nil {
			t.Fatal("unchecked row passed")
		}
	}
}

func TestRelayPartialMissingImportedAddressSkipsWithoutAdding(t *testing.T) {
	planned, checks := relayPartialFixture(2)
	checks.add = func(context.Context, string, string, int) error { t.Error("unexpected add"); return nil }
	kept, report, err := prepareUsableRelayItems(context.Background(), planned, map[string]bool{planned[1].IPv6: true}, false, checks)
	if err != nil || len(kept) != 1 || kept[0].SourceRow != 2 || report.Skipped[0].Row != 1 {
		t.Fatalf("bad selection: %+v %v", report, err)
	}
}

func TestRelayPartialCleanupFailureIsFatalAndRetriable(t *testing.T) {
	planned, checks := relayPartialFixture(2)
	checks.ipv6 = func(_ context.Context, ip netip.Addr) error {
		if ip.String() == planned[0].IPv6 {
			return errors.New("unreachable")
		}
		return nil
	}
	kept, _, err := prepareUsableRelayItems(context.Background(), planned, nil, true, checks)
	if err != nil {
		t.Fatal(err)
	}
	err = cleanupSkippedRelayAddresses(planned, kept, func(string, string, int) error { return errors.New("delete failed") })
	if err == nil || !planned[0].AddedByUs || !planned[1].AddedByUs {
		t.Fatal("cleanup failure discarded ownership")
	}
}

func TestRelayPartialUncheckedCreatesAll500WithoutPublicProbes(t *testing.T) {
	planned, checks := relayPartialFixture(500)
	checks.skipEgress = true
	checks.ipv6 = func(context.Context, netip.Addr) error {
		t.Error("unexpected IPv6 probe")
		return errors.New("unreachable")
	}
	checks.ipv4 = func(context.Context, RelayUpstream) error {
		t.Error("unexpected IPv4 probe")
		return errors.New("unreachable")
	}
	checks.settle = func(context.Context) error { t.Error("unnecessary probe settling delay"); return nil }
	kept, report, err := prepareUsableRelayItems(context.Background(), planned, nil, true, checks)
	if err != nil || len(kept) != 500 || len(report.Skipped) != 0 {
		t.Fatalf("kept=%d report=%+v err=%v", len(kept), report, err)
	}
	for i, item := range kept {
		if item.SourceRow != i+1 || item.IPv6 != planned[i].IPv6 || relayItemUpstream(item) != relayItemUpstream(planned[i]) || !item.AddedByUs || item.EgressCheck != "not_checked" {
			t.Fatalf("unchecked row changed: %+v", item)
		}
	}
}

func TestRelayPartialUncheckedStillRequiresLocalAddresses(t *testing.T) {
	planned, checks := relayPartialFixture(2)
	checks.skipEgress = true
	checks.ready = func(_ context.Context, items []model.RelayItem) ([]error, error) {
		return []error{nil, errors.New("dadfailed")}, nil
	}
	kept, report, err := prepareUsableRelayItems(context.Background(), planned, nil, true, checks)
	if err != nil || len(kept) != 1 || report.Skipped[0].Stage != "dad" || kept[0].EgressCheck != "not_checked" {
		t.Fatalf("local check lost: %+v %v", report, err)
	}
}
