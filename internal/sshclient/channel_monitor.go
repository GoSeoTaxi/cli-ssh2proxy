package sshclient

import (
	"time"

	"go.uber.org/zap"
)

const periodChannelStat = 30 * time.Second

func StartChannelMonitor(r *Reconnector) {
	go func() {
		t := time.NewTicker(periodChannelStat)
		defer t.Stop()
		for range t.C {
			zap.L().Debug("ssh_channels", zap.Int64("current", r.Channels()))
		}
	}()
}
