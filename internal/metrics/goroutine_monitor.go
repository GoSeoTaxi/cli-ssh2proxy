package metrics

import (
	"runtime"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

var bytes int64

func StartGoroutineMonitor(timeOutGorutineMonitor time.Duration) {
	go func() {
		t := time.NewTicker(timeOutGorutineMonitor)
		defer t.Stop()
		for range t.C {
			zap.L().Debug("goroutines", zap.Int("count", runtime.NumGoroutine()))
		}
	}()
}

func Add(n int64) { atomic.AddInt64(&bytes, n) }

func Swap() int64 {
	return atomic.SwapInt64(&bytes, 0)
}
