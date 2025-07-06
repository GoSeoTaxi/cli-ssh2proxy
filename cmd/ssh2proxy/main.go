package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/config"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/logger"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/metrics"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/proxy"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/sshclient"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/tun"
)

const (
	keepAliveInterval = 3 * time.Second
	sleepToReconnect  = 5 * time.Second
	timeCloser        = 2 * time.Second
)

func main() {
	cfg := config.Load()
	logger.Init(cfg.Debug)

	var (
		sshCl *sshclient.Reconnector
		dial  sshclient.DialFunc
		err   error
	)

	for {
		sshCl, dial, err = sshclient.New(cfg.Login, cfg.Password, cfg.Server, cfg.Port, cfg.KeyPath)
		if err == nil {
			break
		}
		zap.L().Info("SSH connect failed", zap.Error(err), zap.String("sleep", "10s"))
		time.Sleep(sleepToReconnect)
	}
	defer sshCl.Close()

	sshclient.StartKeepAlive(cfg, sshCl, keepAliveInterval)
	rawDial := sshclient.WrapTimeout(dial)

	dialCount := func(ctx context.Context, n, a string) (net.Conn, error) {
		conn, e := rawDial(ctx, n, a)
		if e != nil {
			return nil, e
		}
		return metrics.NewTrackConn(&metrics.CountConn{Conn: conn}), nil
	}

	var httpSrv *http.Server
	if cfg.HTTPL != "" {
		httpSrv = proxy.NewHTTP(cfg.HTTPL, dialCount)
	}

	var socksSrv *proxy.SocksServer
	if cfg.SocksL != "" {
		socksSrv, err = proxy.NewSOCKS(cfg.SocksL, dialCount)
		if err != nil {
			zap.L().Fatal("SOCKS", zap.Error(err))
		}
	}

	var cmdTun *exec.Cmd
	if cfg.UseTUN {
		cmdTun, err = tun.RunExternal(cfg.SocksL)
		if err != nil {
			zap.L().Fatal("tun2socks external", zap.Error(err))
		}
	}

	metrics.StartNetMonitor()
	metrics.StartGoroutineMonitor()
	metrics.StartOpenConnectionMonitor()
	metrics.StartMemMonitor()
	metrics.StartCPUMonitor()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	zap.L().Info("shutting down…")

	ctx, cancel := context.WithTimeout(context.Background(), timeCloser)
	defer cancel()

	if httpSrv != nil {
		_ = httpSrv.Shutdown(ctx)
	}
	if socksSrv != nil {
		_ = socksSrv.Shutdown(ctx)
	}
	if cmdTun != nil && cmdTun.Process != nil {
		_ = cmdTun.Process.Kill()
	}

}
