package metrics

import (
	"net"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

const periodOpenStat = 30 * time.Second

var openConns int64

type TrackConn struct {
	net.Conn
	closed int32
}

func NewTrackConn(c net.Conn) *TrackConn {
	atomic.AddInt64(&openConns, 1)
	return &TrackConn{Conn: c}
}

func (c *TrackConn) Close() error {
	if atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		atomic.AddInt64(&openConns, -1)
	}
	return c.Conn.Close()
}

func StartOpenConnectionMonitor() {
	go func() {
		t := time.NewTicker(periodOpenStat)
		defer t.Stop()
		for range t.C {
			cur := atomic.LoadInt64(&openConns)
			zap.L().Debug("open_connections", zap.Int64("current", cur))
		}
	}()
}
