package sshclient

import (
	"time"

	"go.uber.org/zap"
)

const periodChannelStat = 30 * time.Second

func StartChannelMonitor(r *Reconnector) {
	if r.lifecycleErr() != nil {
		return
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		t := time.NewTicker(periodChannelStat)
		defer t.Stop()

		for {
			select {
			case <-r.ctx.Done():
				return
			case <-t.C:
				zap.L().Debug("ssh_channels", zap.Int64("current", r.Channels()))
			}
		}
	}()
}
