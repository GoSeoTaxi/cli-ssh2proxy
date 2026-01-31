package metrics

import (
	"net"
	"sync"
	"time"
)

type IdleConn struct {
	net.Conn
	idleTO    time.Duration
	lastIO    chan struct{}
	shutdown  chan struct{}
	closeOnce sync.Once
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
			_ = c.close()
			return
		case <-c.shutdown:
			return
		}
	}
}

func (c *IdleConn) Close() error {
	return c.close()
}

func (c *IdleConn) close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.shutdown)
		err = c.Conn.Close()
	})
	return err
}
