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
	slotTimeOutHardWaitSlot = 2 * time.Second
	sshConnTimeout          = 5 * time.Second
)

var ErrReconnectorClosed = errors.New("ssh: reconnector closed")

type HostResolver func(ctx context.Context, host string) (net.IP, error)

type ReconnectorOptions struct {
	MaxChannels int64
	EnableProbe bool
}

type Reconnector struct {
	host    string
	port    string
	cfg     *ssh.ClientConfig
	resolve HostResolver
	ctx     context.Context
	cancel  context.CancelFunc

	mu       sync.RWMutex
	client   *ssh.Client
	chanCnt  int64
	maxChans int64
	slotSem  chan struct{}
	probeOn  bool

	reconFlag int32
	wg        sync.WaitGroup
	closeOnce sync.Once
}

func NewReconnector(host, port string, cfg *ssh.ClientConfig, resolver HostResolver, opts ReconnectorOptions) (*Reconnector, error) {
	if resolver == nil {
		resolver = defaultHostResolver
	}
	maxChans := opts.MaxChannels
	if maxChans < 0 {
		maxChans = 0
	}
	lifecycleCtx, cancel := context.WithCancel(context.Background())

	r := &Reconnector{
		host:     host,
		port:     port,
		cfg:      cfg,
		resolve:  resolver,
		ctx:      lifecycleCtx,
		cancel:   cancel,
		maxChans: maxChans,
		probeOn:  opts.EnableProbe,
	}
	r.resetSlotSemaphore()

	cl, resolvedAddr, err := r.connect(r.ctx)
	if err != nil {
		cancel()
		zap.L().Warn("ssh_up_err",
			zap.String("target", r.target()),
			zap.String("resolved_addr", resolvedAddr),
			zap.Error(err),
		)
		return nil, err
	}
	r.client = cl
	r.startConnMonitor()

	r.logProbeResult(cl, "ssh_up_probe")

	zap.L().Info("ssh_up",
		zap.String("target", r.target()),
		zap.String("resolved_addr", resolvedAddr),
		zap.Int64("max_channels", r.maxChans),
		zap.Bool("probe_enabled", r.probeOn),
	)
	return r, nil
}

func (r *Reconnector) Dial(ctx context.Context, n, a string) (net.Conn, error) {
	if err := r.lifecycleErr(); err != nil {
		return nil, err
	}

	slotSem, err := r.waitForSlot(ctx)
	if err != nil {
		return nil, err
	}
	slotHeld := true
	defer func() {
		if slotHeld {
			releaseSlot(slotSem)
		}
	}()

	for i := 0; i < countAttemptsDial; i++ {
		if err := r.lifecycleErr(); err != nil {
			return nil, err
		}

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
			slotHeld = false
			return &channelConn{Conn: conn, rec: r, slotSem: slotSem}, nil
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
			recErr := r.reconnect()
			if recErr == nil {
				continue
			}
			return nil, recErr
		}

		return nil, err
	}
	return nil, errors.New("ssh: reconnect failed")
}

func (r *Reconnector) Close() {
	r.closeOnce.Do(func() {
		r.cancel()

		r.mu.Lock()
		if r.client != nil {
			_ = r.client.Close()
			r.client = nil
			zap.L().Info("ssh_down", zap.String("target", r.target()))
		}
		r.resetSlotSemaphore()
		r.mu.Unlock()

		r.wg.Wait()
	})
}

func (r *Reconnector) reconnect() error {
	if err := r.lifecycleErr(); err != nil {
		return err
	}

	if !atomic.CompareAndSwapInt32(&r.reconFlag, 0, 1) {
		t := time.NewTicker(10 * time.Millisecond)
		defer t.Stop()

		for atomic.LoadInt32(&r.reconFlag) == 1 {
			select {
			case <-r.ctx.Done():
				return ErrReconnectorClosed
			case <-t.C:
			}
		}
		if err := r.lifecycleErr(); err != nil {
			return err
		}
		return nil
	}
	defer atomic.StoreInt32(&r.reconFlag, 0)

	r.mu.Lock()
	if r.client != nil {
		_ = r.client.Close()
		r.client = nil
		zap.L().Info("ssh_down", zap.String("target", r.target()))
	}
	r.mu.Unlock()

	backoff := timeOutBackoff
	for attempt := 0; attempt < countAttemptsDial; attempt++ {
		if err := r.lifecycleErr(); err != nil {
			return err
		}

		cl, resolvedAddr, err := r.connect(r.ctx)
		if err != nil {
			if r.ctx.Err() != nil {
				return ErrReconnectorClosed
			}

			zap.L().Warn("ssh_reconnect_err",
				zap.String("target", r.target()),
				zap.String("resolved_addr", resolvedAddr),
				zap.Int("attempt", attempt+1),
				zap.Duration("backoff", backoff),
				zap.Error(err),
			)

			t := time.NewTimer(backoff)
			select {
			case <-r.ctx.Done():
				t.Stop()
				return ErrReconnectorClosed
			case <-t.C:
			}
			backoff *= 2
			continue
		}

		r.mu.Lock()
		if r.ctx.Err() != nil {
			r.mu.Unlock()
			_ = cl.Close()
			return ErrReconnectorClosed
		}
		r.client = cl
		r.resetSlotSemaphore()
		r.mu.Unlock()

		r.logProbeResult(cl, "ssh_reconnect_probe")

		atomic.StoreInt64(&r.chanCnt, 0)

		zap.L().Info("ssh_reconnect_ok",
			zap.String("target", r.target()),
			zap.String("resolved_addr", resolvedAddr),
			zap.Int("attempt", attempt+1),
			zap.Duration("backoff_used", backoff/2),
			zap.Int64("max_channels", r.maxChans),
		)
		return nil
	}
	if r.ctx.Err() != nil {
		return ErrReconnectorClosed
	}
	zap.L().Error("ssh_reconnect_failed", zap.String("target", r.target()), zap.Int("attempts", countAttemptsDial))
	return errors.New("ssh: retries exceeded")
}

func (r *Reconnector) connect(ctx context.Context) (*ssh.Client, string, error) {
	connectCtx, cancel := context.WithTimeout(ctx, sshConnTimeout)
	defer cancel()

	addr, err := r.resolveAddr(connectCtx)
	if err != nil {
		return nil, "", err
	}
	cl, err := dialSSH(connectCtx, addr, r.cfg)
	if err != nil {
		return nil, addr, err
	}
	return cl, addr, nil
}

func dialSSH(ctx context.Context, addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	d := net.Dialer{Timeout: sshConnTimeout}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	_ = raw.SetDeadline(time.Now().Add(sshConnTimeout))
	cc, chans, reqs, err := ssh.NewClientConn(raw, addr, cfg)
	_ = raw.SetDeadline(time.Time{})
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	return ssh.NewClient(cc, chans, reqs), nil
}

func (r *Reconnector) resolveAddr(ctx context.Context) (string, error) {
	if ip := net.ParseIP(r.host); ip != nil {
		return net.JoinHostPort(ip.String(), r.port), nil
	}
	ip, err := r.resolve(ctx, r.host)
	if err != nil {
		return "", err
	}
	if ip == nil {
		return "", errors.New("ssh: resolver returned nil IP")
	}
	return net.JoinHostPort(ip.String(), r.port), nil
}

func defaultHostResolver(ctx context.Context, host string) (net.IP, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("ssh: hostname resolved to empty list")
	}
	return ips[0], nil
}

func (r *Reconnector) target() string {
	return net.JoinHostPort(r.host, r.port)
}

func (r *Reconnector) logProbeResult(cl *ssh.Client, event string) {
	if !r.probeOn {
		return
	}
	zap.L().Info(event,
		zap.String("target", r.target()),
		zap.Int64("probe_limit", probeMaxChannels(cl)),
	)
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
	rec     *Reconnector
	slotSem chan struct{}
	closed  uint32
}

func (c *channelConn) Close() error {
	if atomic.CompareAndSwapUint32(&c.closed, 0, 1) {
		decrementIfPositive(&c.rec.chanCnt)
		releaseSlot(c.slotSem)
	}
	return c.Conn.Close()
}

func (r *Reconnector) Channels() int64 {
	return atomic.LoadInt64(&r.chanCnt)
}

func (r *Reconnector) waitForSlot(ctx context.Context) (chan struct{}, error) {
	if err := r.lifecycleErr(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	sem := r.slotSem
	r.mu.RUnlock()
	if sem == nil {
		return nil, nil
	}

	deadline := time.NewTimer(slotTimeOutHardWaitSlot)
	defer deadline.Stop()

	select {
	case sem <- struct{}{}:
		return sem, nil
	case <-r.ctx.Done():
		return nil, ErrReconnectorClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-deadline.C:
		return nil, errors.New("ssh: slot wait timeout")
	}
}

func (r *Reconnector) lifecycleErr() error {
	if r.ctx.Err() != nil {
		return ErrReconnectorClosed
	}
	return nil
}

func releaseSlot(sem chan struct{}) {
	if sem == nil {
		return
	}
	select {
	case <-sem:
	default:
	}
}

func (r *Reconnector) resetSlotSemaphore() {
	if r.maxChans <= 0 {
		r.slotSem = nil
		return
	}
	r.slotSem = make(chan struct{}, int(r.maxChans))
}

func decrementIfPositive(counter *int64) {
	for {
		cur := atomic.LoadInt64(counter)
		if cur <= 0 {
			return
		}
		if atomic.CompareAndSwapInt64(counter, cur, cur-1) {
			return
		}
	}
}
