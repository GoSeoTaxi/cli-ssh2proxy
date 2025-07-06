// internal/sshclient/timeout_dial.go
package sshclient

import (
	"context"
	"net"
	"time"
)

const upstreamTimeout = 15 * time.Second

func WrapTimeout(d DialFunc) DialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		c, cancel := context.WithTimeout(ctx, upstreamTimeout)
		defer cancel()
		return d(c, network, addr)
	}
}
