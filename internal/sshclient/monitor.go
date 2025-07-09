package sshclient

import (
	"time"

	"go.uber.org/zap"
)

func (r *Reconnector) startConnMonitor() {
	go func() {
		for {
			r.mu.RLock()
			cl := r.client
			r.mu.RUnlock()

			if cl == nil {
				time.Sleep(200 * time.Millisecond)
				continue
			}

			if err := cl.Wait(); err != nil {
				zap.L().Warn("ssh_conn_lost", zap.Error(err))
			} else {
				zap.L().Warn("ssh_conn_closed")
			}

			if err := r.reconnect(); err != nil {
				zap.L().Warn("ssh_reconnect_failed", zap.Error(err))
			}

		}
	}()
}
