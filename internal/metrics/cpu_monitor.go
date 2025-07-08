package metrics

import (
	"os"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"go.uber.org/zap"
)

func StartCPUMonitor(periodCPUStat time.Duration) {
	go func() {
		p, err := process.NewProcess(int32(os.Getpid()))
		if err != nil {
			zap.L().Warn("cpu-monitor: cannot create proc handle", zap.Error(err))
			return
		}

		t := time.NewTicker(periodCPUStat)
		defer t.Stop()

		_, _ = p.CPUPercent()

		for range t.C {
			if pct, e := p.CPUPercent(); e == nil {
				zap.L().Debug("cpu",
					zap.Float64("percent", pct),
				)
			} else {
				zap.L().Warn("cpu-monitor: read error", zap.Error(e))
			}
		}
	}()
}
