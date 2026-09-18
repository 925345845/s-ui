package service

import (
	"sync"
	"time"

	"github.com/Hhz0823/1s-ui/logger"
)

// Only one relay mutation runs at a time. Keep just the last creation status,
// scoped to its actor and request ID, with no credentials or address lists.
type RelayCreateProgress struct {
	RequestID string `json:"request_id"`
	Stage     string `json:"stage"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	StartedAt int64  `json:"started_at"`
	Active    bool   `json:"active"`
	PoolID    uint   `json:"pool_id,omitempty"`
}

var relayCreationStatus struct {
	sync.RWMutex
	actor string
	value RelayCreateProgress
}

func beginRelayProgress(actor, requestID string) {
	relayCreationStatus.Lock()
	relayCreationStatus.actor = actor
	relayCreationStatus.value = RelayCreateProgress{RequestID: requestID, Stage: "preparing", StartedAt: time.Now().Unix(), Active: true}
	relayCreationStatus.Unlock()
}

func setRelayProgress(stage string, completed, total int) {
	relayCreationStatus.Lock()
	changed := relayCreationStatus.value.Stage != stage
	relayCreationStatus.value.Stage = stage
	relayCreationStatus.value.Completed = completed
	relayCreationStatus.value.Total = total
	started := relayCreationStatus.value.StartedAt
	relayCreationStatus.Unlock()
	if changed && logger.GetLogger() != nil {
		logger.Infof("relay create stage=%s completed=%d/%d elapsed=%ds", stage, completed, total, time.Now().Unix()-started)
	}
}

func finishRelayProgress(poolID uint, err error) {
	stage := "done"
	if err != nil {
		stage = "failed"
	}
	setRelayProgress(stage, 0, 0)
	relayCreationStatus.Lock()
	relayCreationStatus.value.Active = false
	relayCreationStatus.value.PoolID = poolID
	relayCreationStatus.Unlock()
}

func GetRelayCreateProgress(actor, requestID string) *RelayCreateProgress {
	relayCreationStatus.RLock()
	defer relayCreationStatus.RUnlock()
	if actor != relayCreationStatus.actor || requestID == "" || requestID != relayCreationStatus.value.RequestID {
		return nil
	}
	value := relayCreationStatus.value
	return &value
}
