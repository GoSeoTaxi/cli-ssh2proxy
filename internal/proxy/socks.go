package proxy

import (
	"context"
	"io"
	"log"
	"net"

	"github.com/armon/go-socks5"
	"go.uber.org/zap"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/sshclient"
)

const (
	dsnCF = "1.1.1.1:53"
	dnsG  = "8.8.8.8:53"
)

type SocksServer struct {
	listen string
	srv    *socks5.Server
	ln     net.Listener
}

func NewSOCKS(listen string, dial sshclient.DialFunc) (*SocksServer, error) {

	dnsR := newDNSResolver([]string{dsnCF, dnsG}, dial)

	srv, e := socks5.New(&socks5.Config{
		Dial:     dial,
		Resolver: dnsR,
		Logger:   log.New(io.Discard, "", 0),
	})
	if e != nil {
		return nil, e
	}

	ln, e := net.Listen("tcp", listen)
	if e != nil {
		return nil, e
	}

	ss := &SocksServer{listen, srv, ln}
	go func() {
		zap.L().Info("SOCKS proxy listening on", zap.String("listen", listen))
		if err := srv.Serve(ln); err != nil {
			zap.L().Fatal("SOCKS5 proxy Serve error", zap.Error(err))
		}
	}()
	return ss, nil
}

func (s *SocksServer) Shutdown(_ context.Context) error {
	return s.ln.Close()
}
