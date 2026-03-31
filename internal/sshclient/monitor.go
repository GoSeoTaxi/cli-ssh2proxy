package sshclient

import (
	"errors"
	"time"

	"go.uber.org/zap"
)

func (r *Reconnector) startConnMonitor() {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		for {
			select {
			case <-r.ctx.Done():
				return
			default:
			}

			r.mu.RLock()
			cl := r.client
			r.mu.RUnlock()

			if cl == nil {
				t := time.NewTimer(200 * time.Millisecond)
				select {
				case <-r.ctx.Done():
					t.Stop()
					return
				case <-t.C:
				}
				continue
			}

			if err := cl.Wait(); err != nil {
				if r.lifecycleErr() == nil {
					zap.L().Warn("ssh_conn_lost", zap.Error(err))
				}
			} else {
				zap.L().Warn("ssh_conn_closed")
			}

			if r.lifecycleErr() != nil {
				return
			}

			if err := r.reconnect(); err != nil {
				if !errors.Is(err, ErrReconnectorClosed) {
					zap.L().Warn("ssh_reconnect_failed", zap.Error(err))
				}
			}
		}
	}()
}
