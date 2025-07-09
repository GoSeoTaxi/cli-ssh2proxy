package sshclient

import (
	"time"

	"go.uber.org/zap"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/config"
)

func StartKeepAlive(cfg *config.Config, r *Reconnector, interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()

		for range t.C {
			r.mu.RLock()
			cl := r.client
			r.mu.RUnlock()

			if cl == nil {
				zap.L().Debug("keepalive: no client")
				if err := r.reconnect(); err != nil {
					zap.L().Warn("keepalive: reconnect error", zap.Error(err))
				}
				continue
			}

			const timeout = 10 * time.Second

			done := make(chan error, 1)
			go func() {
				_, _, err := cl.SendRequest("keepalive@openssh.com", true, nil)
				done <- err
			}()

			select {
			case err := <-done:
				if err != nil {
					zap.L().Warn("keepalive: send error", zap.Error(err))
					_ = r.reconnect()
				}
			case <-time.After(timeout):
				zap.L().Warn("keepalive: timeout", zap.Duration("after", timeout))
				_ = r.reconnect()
			}
		}
	}()
}
