package sshclient

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

const (
	timeOutBackoff          = 1100 * time.Millisecond
	countAttemptsDial       = 3
	timeOutWaitSlot         = 20 * time.Millisecond
	slotTimeOutHardWaitSlot = 2 * time.Second
	sshConnTimeout          = 5 * time.Second
)

type Reconnector struct {
	addr string
	cfg  *ssh.ClientConfig

	mu       sync.RWMutex
	client   *ssh.Client
	chanCnt  int64
	maxChans int64

	reconFlag int32
}

func NewReconnector(addr string, cfg *ssh.ClientConfig) (*Reconnector, error) {
	cl, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		zap.L().Warn("ssh_up_err", zap.String("addr", addr), zap.Error(err))
		return nil, err
	}

	r := &Reconnector{addr: addr, cfg: cfg, client: cl}
	r.startConnMonitor()

	r.maxChans = probeMaxChannels(cl)

	zap.L().Info("ssh_up", zap.String("addr", addr), zap.Int64("max_channels", r.maxChans))
	return r, nil
}

func (r *Reconnector) Dial(ctx context.Context, n, a string) (net.Conn, error) {
	if err := r.waitForSlot(ctx); err != nil {
		return nil, err
	}

	for i := 0; i < countAttemptsDial; i++ {
		r.mu.RLock()
		cl := r.client
		r.mu.RUnlock()

		if cl == nil {
			if err := r.reconnect(); err != nil {
				return nil, err
			}
			continue
		}

		conn, err := cl.Dial(n, a)
		if err == nil {
			atomic.AddInt64(&r.chanCnt, 1)
			return &channelConn{Conn: conn, rec: r}, nil
		}

		if ocErr, ok := err.(*ssh.OpenChannelError); ok {
			zap.L().Warn("ssh_channel_open_failed",
				zap.Uint32("reason_code", uint32(ocErr.Reason)),
				zap.String("reason_text", ocErr.Message),
				zap.String("upstream_addr", a),
			)
			return nil, err
		}

		if isNetErr(err) {
			if r.reconnect() == nil {
				continue
			}
			return nil, err
		}

		return nil, err
	}
	return nil, errors.New("ssh: reconnect failed")
}

func (r *Reconnector) Close() {
	r.mu.Lock()
	if r.client != nil {
		_ = r.client.Close()
		r.client = nil
		zap.L().Info("ssh_down", zap.String("addr", r.addr))
	}
	r.mu.Unlock()
}

func (r *Reconnector) reconnect() error {
	if !atomic.CompareAndSwapInt32(&r.reconFlag, 0, 1) {
		for atomic.LoadInt32(&r.reconFlag) == 1 {
			time.Sleep(10 * time.Millisecond)
		}
		return nil
	}
	defer atomic.StoreInt32(&r.reconFlag, 0)

	r.mu.Lock()
	if r.client != nil {
		_ = r.client.Close()
		r.client = nil
		zap.L().Info("ssh_down", zap.String("addr", r.addr))
	}
	r.mu.Unlock()

	backoff := timeOutBackoff
	for attempt := 0; attempt < countAttemptsDial; attempt++ {
		d := net.Dialer{Timeout: sshConnTimeout}
		raw, err := d.Dial("tcp", r.addr)

		if err != nil {
			zap.L().Warn("ssh_reconnect_err", zap.Int("attempt", attempt+1), zap.Duration("backoff", backoff), zap.Error(err))

			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		cc, chans, reqs, err := ssh.NewClientConn(raw, r.addr, r.cfg)
		if err != nil {
			_ = raw.Close()
			zap.L().Warn("ssh_reconnect_err", zap.Int("attempt", attempt+1), zap.Duration("backoff", backoff), zap.Error(err))

			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		cl := ssh.NewClient(cc, chans, reqs)

		r.mu.Lock()
		r.client = cl
		r.mu.Unlock()

		r.maxChans = probeMaxChannels(cl)

		atomic.StoreInt64(&r.chanCnt, 0)

		zap.L().Info("ssh_reconnect_ok", zap.Int("attempt", attempt+1), zap.Duration("backoff_used", backoff/2), zap.Int64("max_channels", r.maxChans))
		return nil
	}
	zap.L().Error("ssh_reconnect_failed", zap.String("addr", r.addr), zap.Int("attempts", countAttemptsDial))
	return errors.New("ssh: retries exceeded")
}

func isNetErr(err error) bool {
	if err == io.EOF {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

type channelConn struct {
	net.Conn
	rec    *Reconnector
	closed uint32
}

func (c *channelConn) Close() error {
	if atomic.CompareAndSwapUint32(&c.closed, 0, 1) {
		atomic.AddInt64(&c.rec.chanCnt, -1)
	}
	return c.Conn.Close()
}

func (r *Reconnector) Channels() int64 {
	return atomic.LoadInt64(&r.chanCnt)
}

func (r *Reconnector) waitForSlot(ctx context.Context) error {
	if r.maxChans == 0 {
		return nil
	}
	deadline := time.NewTimer(slotTimeOutHardWaitSlot)
	defer deadline.Stop()

	for {
		if atomic.LoadInt64(&r.chanCnt) < r.maxChans {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("ssh: slot wait timeout")
		case <-time.After(timeOutWaitSlot):
		}
	}
}
