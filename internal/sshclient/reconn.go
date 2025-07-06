package sshclient

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	timeOutBackoff    = 1 * time.Second
	countAttemptsDial = 1
)

type Reconnector struct {
	addr string
	cfg  *ssh.ClientConfig

	mu     sync.RWMutex
	client *ssh.Client
}

func NewReconnector(addr string, cfg *ssh.ClientConfig) (*Reconnector, error) {
	cl, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, err
	}
	return &Reconnector{addr: addr, cfg: cfg, client: cl}, nil
}

func (r *Reconnector) Dial(_ context.Context, n, a string) (net.Conn, error) {
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
			return conn, nil
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
	}
	r.mu.Unlock()
}

func (r *Reconnector) reconnect() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.client != nil {
		_ = r.client.Close()
		r.client = nil
	}

	backoff := timeOutBackoff
	for attempt := 0; attempt < 5; attempt++ {

		cl, err := ssh.Dial("tcp", r.addr, r.cfg)
		if err != nil {
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		r.client = cl
		return nil
	}

	return errors.New("ssh: retries exceeded")
}

func isNetErr(err error) bool {
	if err == io.EOF {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
