package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/Hhz0823/1s-ui/database/model"
)

func fillTestJob(t *testing.T, count int) (context.Context, *relayFillJob, RelayCreateRequest) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req := RelayCreateRequest{Name: "test", RequestID: "fill-test", PortStart: 30000, Count: count, AppleIDIPv4Only: true}
	for i := 0; i < count; i++ {
		req.Upstreams = append(req.Upstreams, RelayUpstream{Server: fmt.Sprintf("192.0.2.%d", i+1), Port: 1080})
	}
	j := &relayFillJob{actor: "test", cancel: cancel, done: make(chan struct{}), status: RelayFillStatus{RequestID: req.RequestID, Requested: count, Active: true}}
	return ctx, j, req
}

func fillTestPool(req RelayCreateRequest, id uint, rows ...int) *model.RelayPool {
	items := make([]model.RelayItem, len(rows))
	for i, row := range rows {
		items[i].SourceRow = row
	}
	return &model.RelayPool{Id: id, Count: len(items), PortStart: req.PortStart, Items: mustJSON(items)}
}

func fillTestUnavailable() (*model.RelayPool, error) {
	return &model.RelayPool{CreationReport: &model.RelayCreationReport{}}, errors.New("unavailable")
}

func TestRelayFillRetriesUntilAllMatched(t *testing.T) {
	ctx, j, req := fillTestJob(t, 3)
	req.IPv6Addresses = []string{"2001:db8::1", "2001:db8::2", "2001:db8::3"}
	calls, pauses := 0, 0
	runRelayFill(ctx, j, req, func(_ context.Context, round RelayCreateRequest) (*model.RelayPool, error) {
		calls++
		if calls > 25 {
			t.Fatal("failed to finish")
		}
		if !round.AppleIDIPv4Only {
			t.Fatal("Apple ID split lost")
		}
		if calls == 1 && !reflect.DeepEqual(round.IPv6Addresses, req.IPv6Addresses) {
			t.Fatal("supplied addresses lost")
		}
		if calls > 1 && len(round.IPv6Addresses) != 0 {
			t.Fatal("failed addresses reused")
		}
		if calls <= 20 {
			return fillTestUnavailable()
		}
		if calls == 21 {
			return fillTestPool(round, 1, 2), nil
		}
		if !reflect.DeepEqual(round.fillSourceRows, []int{1, 3}) || round.Count != 2 || round.PortStart != 30001 {
			t.Fatalf("already matched upstream was resubmitted: %+v", round)
		}
		if round.Upstreams[0] != req.Upstreams[0] || round.Upstreams[1] != req.Upstreams[2] {
			t.Fatal("upstream shifted")
		}
		if round.fillPoolID != 1 || round.Name != req.Name {
			t.Fatal("round did not target original pool")
		}
		return fillTestPool(round, 1, 1, 3), nil
	}, func(context.Context) error { pauses++; return nil })
	status := j.snapshot()
	if status.Active || status.Stage != "done" || status.Matched != 3 || calls != 22 || pauses != 21 || !reflect.DeepEqual(status.PoolIDs, []uint{1}) {
		t.Fatalf("unexpected completion: %+v calls=%d pauses=%d", status, calls, pauses)
	}
}

func TestRelayFillFairRotationAndBusy(t *testing.T) {
	ctx, j, req := fillTestJob(t, 18)
	for i := 0; i < 18; i++ {
		req.IPv6Addresses = append(req.IPv6Addresses, fmt.Sprintf("2001:db8::%x", i+1))
	}
	calls := 0
	runRelayFill(ctx, j, req, func(_ context.Context, round RelayCreateRequest) (*model.RelayPool, error) {
		calls++
		switch calls {
		case 1:
			return nil, errRelayBusy
		case 2:
			if !reflect.DeepEqual(round.IPv6Addresses, req.IPv6Addresses[:16]) {
				t.Fatal("busy round consumed candidates")
			}
			return fillTestUnavailable()
		case 3:
			if round.fillSourceRows[0] != 17 || round.fillSourceRows[1] != 18 {
				t.Fatal("failed first rows starved later rows")
			}
			if !reflect.DeepEqual(round.IPv6Addresses, req.IPv6Addresses[16:]) {
				t.Fatal("remaining candidates lost")
			}
			return fillTestPool(round, 1, round.fillSourceRows...), nil
		case 4:
			if !reflect.DeepEqual(round.fillSourceRows, []int{15, 16}) {
				t.Fatalf("remaining rows: %v", round.fillSourceRows)
			}
			if round.fillPoolID != 1 {
				t.Fatal("destination changed")
			}
			return fillTestPool(round, 1, round.fillSourceRows...), nil
		default:
			t.Fatal("unexpected round")
			return nil, nil
		}
	}, func(context.Context) error { return nil })
	if s := j.snapshot(); s.Matched != 18 || s.Stage != "done" {
		t.Fatalf("status=%+v", s)
	}
}

func TestRelayFillStopPreservesCommittedPools(t *testing.T) {
	ctx, j, req := fillTestJob(t, 2)
	relayFillManager.Lock()
	previous := relayFillManager.job
	relayFillManager.job = j
	relayFillManager.Unlock()
	t.Cleanup(func() { relayFillManager.Lock(); relayFillManager.job = previous; relayFillManager.Unlock() })
	if GetRelayFillStatus("other", "") != nil {
		t.Fatal("status leaked to another actor")
	}
	if _, err := StopRelayFill("other", req.RequestID); err == nil {
		t.Fatal("cross actor stop accepted")
	}
	if _, err := StopRelayFill("test", "wrong-id"); err == nil {
		t.Fatal("wrong task stopped")
	}
	// Start retries return the existing task before parsing a second request.
	if s, err := (&ConfigService{}).StartRelayFill(RelayCreateRequest{RequestID: req.RequestID}, "test", ""); err != nil || s.RequestID != req.RequestID {
		t.Fatalf("idempotency: %+v %v", s, err)
	}
	calls := 0
	runRelayFill(ctx, j, req, func(ctx context.Context, round RelayCreateRequest) (*model.RelayPool, error) {
		calls++
		if calls == 1 {
			return fillTestPool(round, 7, 1), nil
		}
		if _, err := StopRelayFill("test", req.RequestID); err != nil {
			t.Fatal(err)
		}
		if ctx.Err() == nil {
			t.Fatal("stop did not cancel current round")
		}
		return nil, ctx.Err()
	}, func(context.Context) error { return nil })
	status := GetRelayFillStatus("test", req.RequestID)
	if status.Active || status.Stopping || status.Stage != "stopped" || status.Matched != 1 || !reflect.DeepEqual(status.PoolIDs, []uint{7}) {
		t.Fatalf("committed work lost: %+v", status)
	}
	status.PoolIDs[0] = 99
	if j.snapshot().PoolIDs[0] != 7 {
		t.Fatal("status leaked mutable storage")
	}
}

func TestRelayFillFatalResultsStop(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pool    *model.RelayPool
		err     error
		matched int
	}{
		{"db", nil, errors.New("database error"), 0},
		{"core-after-commit", &model.RelayPool{Id: 1, Count: 1, Items: mustJSON([]model.RelayItem{{SourceRow: 1}})}, errors.New("core error"), 1},
		{"bad-row", &model.RelayPool{Id: 1, Count: 1, Items: mustJSON([]model.RelayItem{{SourceRow: 99}})}, nil, 0},
		{"duplicate-row", &model.RelayPool{Id: 1, Count: 2, Items: mustJSON([]model.RelayItem{{SourceRow: 1}, {SourceRow: 1}})}, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, j, req := fillTestJob(t, 2)
			runRelayFill(ctx, j, req, func(context.Context, RelayCreateRequest) (*model.RelayPool, error) { return tc.pool, tc.err }, func(context.Context) error { t.Fatal("fatal error was retried"); return nil })
			if s := j.snapshot(); s.Active || s.Stage != "failed" || s.Error == "" || s.Matched != tc.matched {
				t.Fatalf("status=%+v", s)
			}
		})
	}
}
