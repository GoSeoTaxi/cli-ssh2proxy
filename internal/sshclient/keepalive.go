package sshclient

import (
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/config"
)

func StartKeepAlive(cfg *config.Config, r *Reconnector, interval time.Duration) {
	_ = cfg

	if r.lifecycleErr() != nil {
		return
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		t := time.NewTicker(interval)
		defer t.Stop()

		for {
			select {
			case <-r.ctx.Done():
				return
			case <-t.C:
			}

			r.mu.RLock()
			cl := r.client
			r.mu.RUnlock()

			if cl == nil {
				zap.L().Debug("keepalive: no client")
				if err := r.reconnect(); err != nil {
					if !errors.Is(err, ErrReconnectorClosed) {
						zap.L().Warn("keepalive: reconnect error", zap.Error(err))
					}
				}
				continue
			}

			const timeout = 10 * time.Second

			done := make(chan error, 1)
			go func() {
				_, _, err := cl.SendRequest("keepalive@openssh.com", true, nil)
				done <- err
			}()

			timeoutTimer := time.NewTimer(timeout)
			select {
			case err := <-done:
				timeoutTimer.Stop()
				if err != nil {
					zap.L().Warn("keepalive: send error", zap.Error(err))
					if recErr := r.reconnect(); recErr != nil && !errors.Is(recErr, ErrReconnectorClosed) {
						zap.L().Warn("keepalive: reconnect error", zap.Error(recErr))
					}
				}
			case <-timeoutTimer.C:
				zap.L().Warn("keepalive: timeout", zap.Duration("after", timeout))
				if recErr := r.reconnect(); recErr != nil && !errors.Is(recErr, ErrReconnectorClosed) {
					zap.L().Warn("keepalive: reconnect error", zap.Error(recErr))
				}
			case <-r.ctx.Done():
				timeoutTimer.Stop()
				return
			}
		}
	}()
}
