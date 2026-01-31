package proxy

import (
	"context"
	"io"
	"log"
	"net"

	"github.com/armon/go-socks5"
	"go.uber.org/zap"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/config"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/sshclient"
)

type SocksServer struct {
	listen string
	srv    *socks5.Server
	ln     net.Listener
	dnsR   *DNSResolver
}

func NewSOCKS(cfg *config.Config, dial sshclient.DialFunc) (*SocksServer, error) {

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

	ss := &SocksServer{cfg.SocksL, srv, ln, dnsR}
	go func() {
		zap.L().Info("SOCKS proxy listening on", zap.String("listen", cfg.SocksL))
		if err := srv.Serve(ln); err != nil {
			zap.L().Fatal("SOCKS5 proxy Serve error", zap.Error(err))
		}
	}()
	return ss, nil
}

func (s *SocksServer) Shutdown(_ context.Context) error {
	return s.ln.Close()
}

func (s *SocksServer) DNSCacheLen() int {
	if s == nil || s.dnsR == nil {
		return 0
	}
	return s.dnsR.CacheLen()
}
