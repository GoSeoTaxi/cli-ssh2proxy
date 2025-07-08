// internal/sshclient/probe_maxchans.go
package sshclient

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

func probeMaxChannels(cl *ssh.Client) int64 {
	const (
		safetyCap    = 512
		workerCnt    = safetyCap / 2
		probeTimeout = 10 * time.Second
		singleTO     = 2000 * time.Millisecond
	)

	targets := []string{
		"1.1.1.1:443",
		"8.8.8.8:443",
	}

	type token struct{}
	taskCh := make(chan token, safetyCap)
	stopCh := make(chan struct{})

	var openedMu sync.Mutex
	var opened []net.Conn // все успешно открытые каналы
	var openedCnt int64   // через atomic – чтобы быстро проверять лимит

	// ---------------- workers ----------------
	var wg sync.WaitGroup
	wg.Add(workerCnt)

	dialOnce := func(tgt string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(context.Background(), singleTO)
		defer cancel()
		return cl.DialContext(ctx, "tcp", tgt)
	}

	for w := 0; w < workerCnt; w++ {
		go func(workerID int) {
			defer wg.Done()
			for range taskCh {
				// досрочно вышли?
				if atomic.LoadInt64(&openedCnt) >= safetyCap {
					return
				}

				tgt := targets[workerID%len(targets)] // пусть каждый воркер бьёт в свой tgt
				c, err := dialOnce(tgt)

				switch {
				case err == nil:
					openedMu.Lock()
					opened = append(opened, c)
					openedMu.Unlock()
					atomic.AddInt64(&openedCnt, 1)

				case func() bool { _, ok := err.(*ssh.OpenChannelError); return ok }():
					// достигнут потолок – тушим всё
					close(stopCh)
					return

				default:
					// что-то другое (network / timeout / EOF) – прекращаем probe
					close(stopCh)
					return
				}
			}
		}(w)
	}

	// ---------------- producer ----------------
	go func() {
		defer close(taskCh)
		for i := 0; i < safetyCap; i++ {
			select {
			case <-stopCh:
				return
			default:
				taskCh <- token{}
			}
		}
	}()

	// ждём либо таймаут всего probe, либо завершение worker-ов
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(probeTimeout):
		zap.L().Warn("probe_max_channels_timeout", zap.Duration("timeout", probeTimeout))
	}

	// прибираем за собой
	for _, c := range opened {
		_ = c.Close()
	}
	return atomic.LoadInt64(&openedCnt)
}
