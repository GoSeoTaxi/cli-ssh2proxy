package limits

import (
	"io"
	"net"
	"time"

	"go.uber.org/zap"
)

type RejectAction int

const (
	RejectDrop RejectAction = iota
	RejectHTTP503
	RejectHTTP429
)

type ListenerOptions struct {
	Protocol        string
	SessionLimiter  *SessionLimiter
	RateLimiter     *IPRateLimiter
	OverLimitAction RejectAction
	RateLimitAction RejectAction
	RejectWriteWait time.Duration
}

func WrapListener(ln net.Listener, opts ListenerOptions) net.Listener {
	if ln == nil {
		return nil
	}
	if opts.RejectWriteWait <= 0 {
		opts.RejectWriteWait = 200 * time.Millisecond
	}
	return &limitedListener{
		Listener: ln,
		opts:     opts,
	}
}

type limitedListener struct {
	net.Listener
	opts ListenerOptions
}

func (l *limitedListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}

		clientIP := ClientIPFromRemoteAddr(conn.RemoteAddr())
		if !l.opts.RateLimiter.Allow(conn.RemoteAddr()) {
			zap.L().Warn("client_handshake_rate_limited",
				zap.String("protocol", l.opts.Protocol),
				zap.String("client_ip", clientIP),
			)
			rejectConn(conn, l.opts.RateLimitAction, l.opts.RejectWriteWait)
			continue
		}

		if !l.opts.SessionLimiter.Acquire() {
			zap.L().Warn("client_connection_limited",
				zap.String("protocol", l.opts.Protocol),
				zap.String("client_ip", clientIP),
				zap.Int64("max", l.opts.SessionLimiter.Max()),
				zap.Int64("in_use", l.opts.SessionLimiter.InUse()),
			)
			rejectConn(conn, l.opts.OverLimitAction, l.opts.RejectWriteWait)
			continue
		}

		return WrapConnWithRelease(conn, l.opts.SessionLimiter.Release), nil
	}
}

func rejectConn(conn net.Conn, action RejectAction, deadline time.Duration) {
	if conn == nil {
		return
	}
	switch action {
	case RejectHTTP503:
		writeHTTPReject(conn, "HTTP/1.1 503 Service Unavailable\r\nConnection: close\r\nContent-Length: 0\r\n\r\n", deadline)
	case RejectHTTP429:
		writeHTTPReject(conn, "HTTP/1.1 429 Too Many Requests\r\nConnection: close\r\nRetry-After: 1\r\nContent-Length: 0\r\n\r\n", deadline)
	default:
		_ = conn.Close()
	}
}

func writeHTTPReject(conn net.Conn, payload string, deadline time.Duration) {
	_ = conn.SetWriteDeadline(time.Now().Add(deadline))
	_, _ = io.WriteString(conn, payload)
	_ = conn.SetWriteDeadline(time.Time{})
	_ = conn.Close()
}
