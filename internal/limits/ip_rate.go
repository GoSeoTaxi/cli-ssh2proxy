package limits

import (
	"net"
	"sync"
	"time"
)

const minCleanupInterval = 10 * time.Second

type IPRateLimiter struct {
	mu            sync.Mutex
	buckets       map[string]*ipBucket
	ratePerSecond float64
	burst         float64
	ttl           time.Duration
	cleanupEvery  time.Duration
	lastCleanup   time.Time
	enabled       bool
	now           func() time.Time
}

type ipBucket struct {
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

func NewIPRateLimiter(rps int64, burst int64, ttl time.Duration) *IPRateLimiter {
	l := &IPRateLimiter{
		now: time.Now,
	}
	if rps <= 0 || burst <= 0 {
		return l
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	cleanupEvery := ttl / 2
	if cleanupEvery < minCleanupInterval {
		cleanupEvery = minCleanupInterval
	}

	l.buckets = make(map[string]*ipBucket)
	l.ratePerSecond = float64(rps)
	l.burst = float64(burst)
	l.ttl = ttl
	l.cleanupEvery = cleanupEvery
	l.enabled = true
	return l
}

func (l *IPRateLimiter) Allow(remoteAddr net.Addr) bool {
	if l == nil || !l.enabled {
		return true
	}
	ip := ClientIPFromRemoteAddr(remoteAddr)
	if ip == "" {
		ip = "<unknown>"
	}

	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastCleanup.IsZero() || now.Sub(l.lastCleanup) >= l.cleanupEvery {
		for key, b := range l.buckets {
			if now.Sub(b.lastSeen) >= l.ttl {
				delete(l.buckets, key)
			}
		}
		l.lastCleanup = now
	}

	b, ok := l.buckets[ip]
	if !ok {
		b = &ipBucket{
			tokens:     l.burst,
			lastRefill: now,
			lastSeen:   now,
		}
		l.buckets[ip] = b
	}

	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * l.ratePerSecond
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.lastRefill = now
	}
	b.lastSeen = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *IPRateLimiter) Enabled() bool {
	return l != nil && l.enabled
}
