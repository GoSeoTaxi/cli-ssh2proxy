package metrics

import (
	"net"
	"time"

	"go.uber.org/zap"
)

const timeOutNetStats = 10 * time.Second

func StartNetMonitor() {
	go func() {
		for {
			now := time.Now()
			next := now.Truncate(timeOutNetStats).Add(timeOutNetStats)
			time.Sleep(time.Until(next))

			b := Swap()

			seconds := timeOutNetStats.Seconds()
			mbps := float64(b*8) / (seconds * 1e6)

			zap.L().Debug("traffic", zap.Float64("mbps", mbps), zap.Int64("bytes", b))
		}
	}()
}

type CountConn struct{ net.Conn }

func (c *CountConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	Add(int64(n))
	return n, err
}
func (c *CountConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	Add(int64(n))
	return n, err
}
