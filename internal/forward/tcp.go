package forward

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/sshclient"
)

type TCPForwarder struct {
	ln   net.Listener
	wg   sync.WaitGroup
	stop chan struct{}
}

func StartTCP(localListen, remoteAddr string, dial sshclient.DialFunc) (*TCPForwarder, error) {
	ln, err := net.Listen("tcp", localListen)
	if err != nil {
		return nil, err
	}

	f := &TCPForwarder{ln: ln, stop: make(chan struct{})}
	go func() {
		zap.L().Info("tcp_forwarder up", zap.String("local", localListen), zap.String("remote", remoteAddr))
		for {
			c, err := ln.Accept()
			if err != nil {
				select {
				case <-f.stop:
					return
				default:
					zap.L().Warn("tcp_forwarder accept", zap.Error(err))
					continue
				}
			}
			f.wg.Add(1)
			go func(src net.Conn) {
				defer f.wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				dst, err := dial(ctx, "tcp", remoteAddr)
				if err != nil {
					_ = src.Close()
					zap.L().Warn("tcp_forwarder dial", zap.String("remote", remoteAddr), zap.Error(err))
					return
				}
				pipe := func(a, b net.Conn) {
					defer a.Close()
					defer b.Close()
					_, _ = io.Copy(a, b)
				}
				go pipe(src, dst)
				pipe(dst, src)
			}(c)
		}
	}()
	return f, nil
}

func (f *TCPForwarder) Close() {
	close(f.stop)
	_ = f.ln.Close()
	done := make(chan struct{})
	go func() { f.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}
