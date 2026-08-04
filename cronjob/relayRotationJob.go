package cronjob

import (
	"time"

	"github.com/Hhz0823/1s-ui/config"
	"github.com/Hhz0823/1s-ui/service"
)

type RelayRotationJob struct {
	service.ConfigService
}

func NewRelayRotationJob() *RelayRotationJob {
	return &RelayRotationJob{}
}

func (s *RelayRotationJob) Run() {
	if config.IsSkipCore() {
		return
	}
	s.ConfigService.RotateDueRelays(time.Now())
}
