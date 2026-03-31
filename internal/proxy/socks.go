package proxy

import (
	"context"
	"errors"
	"io"
	"log"
	"net"

	"github.com/armon/go-socks5"
	"go.uber.org/zap"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/config"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/limits"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/sshclient"
)

type SocksServer struct {
	listen string
	srv    *socks5.Server
	ln     net.Listener
	dnsR   *DNSResolver
	errCh  chan error
}

func NewSOCKS(cfg *config.Config, dial sshclient.DialFunc, sessions *limits.SessionLimiter, handshakes *limits.IPRateLimiter) (*SocksServer, error) {

	dnsR := NewDNSResolver(cfg.DNSServers, cfg.DNSv6, dial)

	conf := &socks5.Config{
		Dial:     dial,
		Resolver: dnsR,
		Logger:   log.New(io.Discard, "", 0),
	}
	if cfg.SocksUser != "" || cfg.SocksPass != "" {
		conf.Credentials = socks5.StaticCredentials{
			cfg.SocksUser: cfg.SocksPass,
		}
	}

	srv, e := socks5.New(conf)
	if e != nil {
		return nil, e
	}

	ln, e := net.Listen("tcp", cfg.SocksL)
	if e != nil {
		return nil, e
	}
	ln = limits.WrapListener(ln, limits.ListenerOptions{
		Protocol:        "socks5",
		SessionLimiter:  sessions,
		RateLimiter:     handshakes,
		OverLimitAction: limits.RejectDrop,
		RateLimitAction: limits.RejectDrop,
	})

	ss := &SocksServer{
		listen: cfg.SocksL,
		srv:    srv,
		ln:     ln,
		dnsR:   dnsR,
		errCh:  make(chan error, 1),
	}
	go func() {
		defer close(ss.errCh)
		zap.L().Info("SOCKS proxy listening on", zap.String("listen", cfg.SocksL))
		if err := srv.Serve(ln); !isSocksServeExit(err) {
			ss.errCh <- err
		}
	}()
	return ss, nil
}

func (s *SocksServer) Shutdown(_ context.Context) error {
	return s.ln.Close()
}

func (s *SocksServer) Errors() <-chan error {
	if s == nil {
		return nil
	}
	return s.errCh
}

func (s *SocksServer) DNSCacheLen() int {
	if s == nil || s.dnsR == nil {
		return 0
	}
	return s.dnsR.CacheLen()
}

func isSocksServeExit(err error) bool {
	return err == nil || errors.Is(err, net.ErrClosed)
}
