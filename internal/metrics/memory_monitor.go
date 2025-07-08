package metrics

import (
	"runtime"
	"time"

	"go.uber.org/zap"
)

func StartMemMonitor(periodMemStat time.Duration) {
	go func() {
		t := time.NewTicker(periodMemStat)
		defer t.Stop()

		var m runtime.MemStats
		for range t.C {
			runtime.ReadMemStats(&m)

			toMB := func(b uint64) float64 { return float64(b) / (1 << 20) }

			zap.L().Debug("memory",
				zap.Float64("alloc_mb", toMB(m.Alloc)),
				zap.Float64("heap_sys_mb", toMB(m.HeapSys)),
				zap.Float64("heap_idle_mb", toMB(m.HeapIdle)),
				zap.Float64("heap_inuse_mb", toMB(m.HeapInuse)),
				zap.Float64("stack_inuse_mb", toMB(m.StackInuse)),
				zap.Uint32("num_gc", m.NumGC),
			)
		}
	}()
}
