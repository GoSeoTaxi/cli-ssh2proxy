package metrics

import (
	"net"
	"time"
)

type IdleConn struct {
	net.Conn
	idleTO   time.Duration
	lastIO   chan struct{}
	shutdown chan struct{}
}

func NewIdleConn(c net.Conn, idle time.Duration) *IdleConn {
	ic := &IdleConn{
		Conn:     c,
		idleTO:   idle,
		lastIO:   make(chan struct{}, 1),
		shutdown: make(chan struct{}),
	}
	go ic.watchdog()
	return ic
}

func (c *IdleConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	select {
	case c.lastIO <- struct{}{}:
	default:
	}
	return n, err
}

func (c *IdleConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	select {
	case c.lastIO <- struct{}{}:
	default:
	}
	return n, err
}

func (c *IdleConn) watchdog() {
	timer := time.NewTimer(c.idleTO)
	defer timer.Stop()
	for {
		select {
		case <-c.lastIO:
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(c.idleTO)
		case <-timer.C:
			_ = c.Conn.Close()
			return
		case <-c.shutdown:
			return
		}
	}
}

func (c *IdleConn) Close() error {
	close(c.shutdown)
	return c.Conn.Close()
}
