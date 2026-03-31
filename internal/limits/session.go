package limits

import "net"

type SessionLimiter struct {
	sem chan struct{}
}

func NewSessionLimiter(max int64) *SessionLimiter {
	if max <= 0 {
		return &SessionLimiter{}
	}
	return &SessionLimiter{sem: make(chan struct{}, int(max))}
}

func (l *SessionLimiter) Acquire() bool {
	if l == nil || l.sem == nil {
		return true
	}
	select {
	case l.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *SessionLimiter) Release() {
	if l == nil || l.sem == nil {
		return
	}
	select {
	case <-l.sem:
	default:
	}
}

func (l *SessionLimiter) Max() int64 {
	if l == nil || l.sem == nil {
		return 0
	}
	return int64(cap(l.sem))
}

func (l *SessionLimiter) InUse() int64 {
	if l == nil || l.sem == nil {
		return 0
	}
	return int64(len(l.sem))
}

func WrapConnWithRelease(conn net.Conn, release func()) net.Conn {
	if conn == nil || release == nil {
		return conn
	}
	return &releaseConn{Conn: conn, release: release}
}
