package metrics

import (
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"go.uber.org/zap"
)

func StartMemMonitor(periodMemStat time.Duration, dnsCacheLen func() int) {
	go func() {
		p, err := process.NewProcess(int32(os.Getpid()))
		if err != nil {
			zap.L().Warn("mem-monitor: cannot create proc handle", zap.Error(err))
			p = nil
		}

		t := time.NewTicker(periodMemStat)
		defer t.Stop()

		var m runtime.MemStats
		toMB := func(b uint64) float64 { return float64(b) / (1 << 20) }
		for range t.C {
			runtime.ReadMemStats(&m)

			fields := make([]zap.Field, 0, 9)
			fields = append(fields,
				zap.Float64("alloc_mb", toMB(m.Alloc)),
				zap.Float64("heap_sys_mb", toMB(m.HeapSys)),
				zap.Float64("heap_idle_mb", toMB(m.HeapIdle)),
				zap.Float64("heap_inuse_mb", toMB(m.HeapInuse)),
				zap.Float64("stack_inuse_mb", toMB(m.StackInuse)),
				zap.Uint32("num_gc", m.NumGC),
			)

			if p != nil {
				if mem, e := p.MemoryInfo(); e == nil {
					fields = append(fields,
						zap.Float64("rss_mb", toMB(mem.RSS)),
						zap.Float64("vms_mb", toMB(mem.VMS)),
					)
				} else {
					zap.L().Warn("mem-monitor: read error", zap.Error(e))
					p = nil
				}
			}

			if dnsCacheLen != nil {
				fields = append(fields, zap.Int("dns_cache_len", dnsCacheLen()))
			}

			zap.L().Info("memory", fields...)
		}
	}()
}
