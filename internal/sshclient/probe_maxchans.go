package sshclient

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

func probeMaxChannels(cl *ssh.Client) int64 {
	const (
		safetyCap      = 512
		workers        = safetyCap / 4
		probeTimeout   = 1 * time.Second
		safeMaxDefault = 64
	)

	targets := []string{
		"1.1.1.1:443",
		"8.8.8.8:443",
	}

	type result struct {
		ok    bool
		limit bool
		fatal bool
	}

	jobs := make(chan int, safetyCap)
	results := make(chan result, safetyCap)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				tgt := targets[idx%len(targets)]

				ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
				conn, err := cl.DialContext(ctx, "tcp", tgt)
				cancel()

				switch e := err.(type) {
				case nil:
					_ = conn.Close()
					results <- result{ok: true}
				case *ssh.OpenChannelError:
					switch e.Reason {
					case ssh.ResourceShortage:
						results <- result{limit: true}
						return
					default:
						results <- result{ok: true}
					}
				default:
					results <- result{fatal: true}
					return
				}
			}
		}()
	}

	for i := 0; i < safetyCap; i++ {
		jobs <- i
	}
	close(jobs)

	go func() { wg.Wait(); close(results) }()

	var okCnt int64
	for r := range results {
		if r.fatal {
			continue
		}
		if r.ok {
			okCnt++
		}
		if r.limit {
			continue
		}
	}

	if okCnt == 0 || okCnt < safeMaxDefault {
		zap.L().Warn("probe_max_channels_failed – using safe default", zap.Int("default", safeMaxDefault))
		return safeMaxDefault
	}
	return okCnt
}
