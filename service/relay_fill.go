package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Hhz0823/1s-ui/database/model"
	"github.com/Hhz0823/1s-ui/util/common"
)

// The runner commits small pools as it goes. No browser connection is held,
// and stopping a job only rolls back the round that has not committed yet.
type RelayFillStatus struct {
	RequestID string `json:"request_id"`
	Requested int    `json:"requested"`
	Matched   int    `json:"matched"`
	Rounds    int    `json:"rounds"`
	Active    bool   `json:"active"`
	Stopping  bool   `json:"stopping"`
	Stage     string `json:"stage"`
	StartedAt int64  `json:"started_at"`
	PoolIDs   []uint `json:"pool_ids"`
	Error     string `json:"error,omitempty"`
}

type relayFillJob struct {
	mu     sync.Mutex
	actor  string
	status RelayFillStatus
	cancel context.CancelFunc
	done   chan struct{}
}

var relayFillManager struct {
	sync.Mutex
	job *relayFillJob
}

func (j *relayFillJob) snapshot() RelayFillStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	value := j.status
	value.PoolIDs = append([]uint{}, value.PoolIDs...)
	return value
}

func GetRelayFillStatus(actor, requestID string) *RelayFillStatus {
	relayFillManager.Lock()
	j := relayFillManager.job
	relayFillManager.Unlock()
	if j == nil || j.actor != actor {
		return nil
	}
	value := j.snapshot()
	if requestID != "" && value.RequestID != requestID {
		return nil
	}
	if value.Active && !value.Stopping {
		if progress := GetRelayCreateProgress(actor, value.RequestID); progress != nil && progress.Active {
			value.Stage = progress.Stage
		}
	}
	return &value
}

func StopRelayFill(actor, requestID string) (*RelayFillStatus, error) {
	relayFillManager.Lock()
	j := relayFillManager.job
	relayFillManager.Unlock()
	if j == nil || j.actor != actor || requestID == "" || j.snapshot().RequestID != requestID {
		return nil, common.NewError("relay fill task was not found")
	}
	j.mu.Lock()
	if j.status.Active {
		j.status.Stopping = true
		j.cancel()
	}
	j.mu.Unlock()
	return GetRelayFillStatus(actor, requestID), nil
}

func (s *ConfigService) StartRelayFill(req RelayCreateRequest, actor, publicHost string) (*RelayFillStatus, error) {
	if req.RequestID == "" || len(req.RequestID) > 64 {
		return nil, common.NewError("invalid relay request ID")
	}
	// Retrying a lost start response must not submit the same upstreams twice.
	if current := GetRelayFillStatus(actor, req.RequestID); current != nil {
		return current, nil
	}
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if !relayModePairsUpstream(req.Mode) {
		return nil, common.NewError("automatic filling requires paired or dualstack mode")
	}
	if len(req.Upstreams) == 0 {
		var err error
		req.Upstreams, err = parseRelayUpstreams(req.UpstreamText)
		if err != nil {
			return nil, err
		}
	}
	req.Count = len(req.Upstreams)
	if req.Count < 1 || req.Count > maxRelayItems {
		return nil, common.NewError("upstream count must be between 1 and 500")
	}
	if req.PortStart < 1 || req.Count > 65536-req.PortStart {
		return nil, common.NewError("relay port range is invalid")
	}
	if req.PasswordLength < 8 || req.PasswordLength > 64 {
		req.PasswordLength = 12
	}
	if req.UsernamePrefix == "" {
		req.UsernamePrefix = "relay"
	}
	base, prefix, iface, err := resolveRelayBase(req)
	if err != nil {
		return nil, err
	}
	req.BaseIPv6, req.Prefix, req.Interface = base.String(), prefix, iface
	if _, err := s.prepareRelayItems(req); err != nil {
		return nil, err
	}
	if req.Name == "" {
		req.Name = "relay-" + common.Random(5)
	}
	verified := true
	req.VerifyEgress = &verified
	req.AddSystemAddresses = true
	req.Source = ""
	req.UpstreamText = ""
	// Own the slice storage used by the background goroutine.
	req.Upstreams = append([]RelayUpstream(nil), req.Upstreams...)
	req.IPv6Addresses = append([]string(nil), req.IPv6Addresses...)
	ctx, cancel := context.WithCancel(context.Background())
	j := &relayFillJob{actor: actor, cancel: cancel, done: make(chan struct{}), status: RelayFillStatus{
		RequestID: req.RequestID, Requested: req.Count, Active: true, Stage: "preparing", StartedAt: time.Now().Unix(), PoolIDs: []uint{},
	}}
	relayFillManager.Lock()
	if previous := relayFillManager.job; previous != nil {
		status := previous.snapshot()
		if previous.actor == actor && status.RequestID == req.RequestID {
			relayFillManager.Unlock()
			cancel()
			return &status, nil
		}
		if status.Active {
			relayFillManager.Unlock()
			cancel()
			return nil, common.NewError("relay filling is already active")
		}
	}
	relayFillManager.job = j
	relayFillManager.Unlock()
	go runRelayFill(ctx, j, req, func(ctx context.Context, round RelayCreateRequest) (*model.RelayPool, error) {
		return s.CreateRelayContext(ctx, round, actor, publicHost)
	}, func(ctx context.Context) error { return waitRelayPoolSettle(ctx, 5*time.Second) })
	value := j.snapshot()
	return &value, nil
}

type relayFillPending struct {
	row      int
	upstream RelayUpstream
}

func runRelayFill(ctx context.Context, j *relayFillJob, req RelayCreateRequest,
	create func(context.Context, RelayCreateRequest) (*model.RelayPool, error), pause func(context.Context) error) {
	defer close(j.done)
	defer j.cancel()
	defer func() {
		j.mu.Lock()
		defer j.mu.Unlock()
		j.status.Active = false
		if j.status.Error != "" {
			j.status.Stage = "failed"
		} else if j.status.Matched == j.status.Requested {
			j.status.Stage = "done"
		} else {
			j.status.Stage = "stopped"
		}
		j.status.Stopping = false
	}()
	pending := make([]relayFillPending, len(req.Upstreams))
	for i, upstream := range req.Upstreams {
		pending[i] = relayFillPending{i + 1, upstream}
	}
	candidates := append([]string(nil), req.IPv6Addresses...)
	for len(pending) > 0 && ctx.Err() == nil {
		n := min(16, len(pending))
		batch := append([]relayFillPending(nil), pending[:n]...)
		round := req
		round.Count = n
		round.Upstreams = make([]RelayUpstream, n)
		round.fillSourceRows = make([]int, n)
		for i, item := range batch {
			round.Upstreams[i] = item.upstream
			round.fillSourceRows[i] = item.row
		}
		count := min(n, len(candidates))
		round.IPv6Addresses = append([]string(nil), candidates[:count]...)
		candidates = candidates[count:]
		j.mu.Lock()
		j.status.Rounds++
		j.status.Stage = "preparing"
		round.Name = fmt.Sprintf("%s-%03d", req.Name, len(j.status.PoolIDs)+1)
		j.mu.Unlock()
		pool, err := create(ctx, round)
		if errors.Is(err, errRelayBusy) {
			candidates = append(round.IPv6Addresses, candidates...)
			if pause(ctx) != nil {
				return
			}
			continue
		}
		matched := make(map[int]bool)
		if pool != nil && pool.Id != 0 {
			var items []model.RelayItem
			if json.Unmarshal(pool.Items, &items) != nil || len(items) != pool.Count || pool.Count == 0 {
				j.mu.Lock()
				j.status.Error = "invalid_result"
				j.mu.Unlock()
				return
			}
			for _, item := range items {
				valid := false
				for _, original := range batch {
					if item.SourceRow == original.row {
						valid = true
						break
					}
				}
				if !valid || matched[item.SourceRow] {
					j.mu.Lock()
					j.status.Error = "invalid_result"
					j.mu.Unlock()
					return
				}
				matched[item.SourceRow] = true
			}
			j.mu.Lock()
			j.status.Matched += len(matched)
			j.status.PoolIDs = append(j.status.PoolIDs, pool.Id)
			j.mu.Unlock()
			req.PortStart = pool.PortStart + pool.Count
			if err != nil {
				j.mu.Lock()
				j.status.Error = "configuration_failed"
				j.mu.Unlock()
				return
			}
		} else if err != nil && ctx.Err() == nil && (pool == nil || pool.CreationReport == nil) {
			// Syntax, DB, core and cleanup errors are not IPv6 availability
			// failures. Stop instead of spinning on an operation that cannot save.
			j.mu.Lock()
			j.status.Error = "configuration_failed"
			j.mu.Unlock()
			return
		}
		// Rotate unfilled upstreams, so one unavailable IPv4 cannot starve all
		// the later rows. Only persisted rows leave the work queue.
		pending = pending[n:]
		for _, item := range batch {
			if !matched[item.row] {
				pending = append(pending, item)
			}
		}
		if len(pending) > 0 && ctx.Err() == nil {
			j.mu.Lock()
			j.status.Stage = "refilling"
			j.mu.Unlock()
			if pause(ctx) != nil {
				return
			}
		}
	}
}
