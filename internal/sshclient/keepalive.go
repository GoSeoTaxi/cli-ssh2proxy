package sshclient

import (
	"log"
	"time"

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
				if cfg.Debug {
					log.Printf("keepalive: no client")
				}
				e := r.reconnect()
				if e != nil && cfg.Debug {
					log.Printf("keepalive: reconnect error: %v", e)
				}
				continue
			}

			_, _, err := cl.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				if cfg.Debug {
					log.Printf("keepalive error: %v", err)
				}
				e := r.reconnect()
				if e != nil && cfg.Debug {
					log.Printf("keepalive: reconnect error: %v", e)
				}
			}
		}
	}()
}
