package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/config"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/forward"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/logger"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/metrics"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/proxy"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/sshclient"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/tun"
)

const (
	keepAliveInterval     = 1 * time.Second
	sleepToReconnect      = 5 * time.Second
	timeCloser            = 2 * time.Second
	timeOutIdleConnection = 30 * time.Second
)

func main() {
	cfg := config.Load()
	logger.Init(cfg.Debug)
	logDotEnv()

	bootDNS := proxy.NewDNSResolver(cfg.DNSServers, cfg.DNSv6, nil)

	var (
		sshCl *sshclient.Reconnector
		dial  sshclient.DialFunc
		err   error
	)

	serverAddr := cfg.Server
	if ip := net.ParseIP(cfg.Server); ip != nil {
		serverAddr = ip.String()
		zap.L().Debug("bootstrap DNS skipped: server is IP", zap.String("server", serverAddr))
	}
	for {
		if net.ParseIP(serverAddr) == nil {
			_, ip, err := bootDNS.ResolveBoot(context.Background(), serverAddr)
			if err != nil {
				zap.L().Info("bootstrap DNS failed", zap.Error(err))
				time.Sleep(sleepToReconnect)
				continue
			}
			serverAddr = ip.String()
		}

		sshCl, dial, err = sshclient.New(cfg.Login, cfg.Password, serverAddr, cfg.Port, cfg.KeyPath)
		if err == nil {
			break
		}
		zap.L().Info("SSH connect failed", zap.Error(err), zap.String("sleep", "10s"))
		time.Sleep(sleepToReconnect)
	}
	defer sshCl.Close()

	sshclient.StartKeepAlive(cfg, sshCl, keepAliveInterval)
	sshclient.StartChannelMonitor(sshCl)

	rawDial := sshclient.WrapTimeout(dial)
	dialCount := func(ctx context.Context, n, a string) (net.Conn, error) {
		if err := sshclient.RejectIPv6(a, cfg.DNSv6); err != nil {
			return nil, err
		}

		raw, e := rawDial(ctx, n, a)
		if e != nil {
			return nil, e
		}

		return metrics.NewTrackConn(&metrics.CountConn{Conn: metrics.NewIdleConn(raw, timeOutIdleConnection)}), nil
	}

	var fwd *forward.TCPForwarder
	fwd, err = forward.StartTCP(cfg.UDPGWLocal, cfg.UDPGWRemote, dialCount)
	if err != nil {
		zap.L().Fatal("udpgw forward", zap.Error(err))
	}
	defer fwd.Close()

	var httpSrv *http.Server
	if cfg.HTTPL != "" {
		httpSrv, err = proxy.NewHTTP(cfg.HTTPL, cfg.HTTPUser, cfg.HTTPPass, dialCount)
		if err != nil {
			zap.L().Fatal("HTTP", zap.Error(err))
		}
	}

	var socksSrv *proxy.SocksServer
	if cfg.SocksL != "" {
		socksSrv, err = proxy.NewSOCKS(cfg, dialCount)
		if err != nil {
			zap.L().Fatal("SOCKS", zap.Error(err))
		}
	}

	var cmdTun *exec.Cmd
	if cfg.UseTUN {
		proxyAddr := cfg.SocksProxy
		if proxyAddr == "" {
			proxyAddr = cfg.SocksL
		}
		cmdTun, err = tun.RunExternal(proxyAddr)
		if err != nil {
			zap.L().Fatal("tun2socks external", zap.Error(err))
		}
	}

	if cfg.TimeOutMonitorIntSec > 0 {
		metrics.StartNetMonitor(cfg.TimeOutMonitor)
		metrics.StartGoroutineMonitor(cfg.TimeOutMonitor)
		metrics.StartOpenConnectionMonitor(cfg.TimeOutMonitor)
		dnsCacheLen := func() int {
			total := 0
			if bootDNS != nil {
				total += bootDNS.CacheLen()
			}
			if socksSrv != nil {
				total += socksSrv.DNSCacheLen()
			}
			return total
		}
		metrics.StartMemMonitor(cfg.TimeOutMonitor, dnsCacheLen)
		metrics.StartCPUMonitor(cfg.TimeOutMonitor)
	} else {
		zap.L().Info("metrics disabled", zap.Int64("timeout_sec", cfg.TimeOutMonitorIntSec))
	}

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

func logDotEnv() {
	if _, err := os.Stat(".env"); err != nil {
		if os.IsNotExist(err) {
			zap.L().Info(".env not found")
			return
		}
		zap.L().Info(".env stat failed", zap.Error(err))
		return
	}

	envMap, err := godotenv.Read()
	if err != nil {
		zap.L().Info(".env read failed", zap.Error(err))
		return
	}

	masked := make(map[string]string, len(envMap))
	keys := make([]string, 0, len(envMap))
	for k, v := range envMap {
		keys = append(keys, k)
		masked[k] = maskEnvValue(k, v)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", k, masked[k]))
	}

	zap.L().Info(".env loaded", zap.Strings("env", lines))
}

func maskEnvValue(key, value string) string {
	if value == "" {
		return ""
	}
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "LOGIN", "PASSWORD", "SERVER", "USER", "USERNAME", "SSH_USER", "SSH_LOGIN":
		if strings.Contains(strings.ToUpper(key), "PASS") {
			return "******"
		}
		return maskMiddle(value)
	case "SOCKS_LSN", "HTTP_LSN":
		return maskListenPassword(value)
	default:
		return value
	}
}

func maskMiddle(value string) string {
	if len(value) <= 2 {
		return "**"
	}
	return value[:1] + strings.Repeat("*", len(value)-2) + value[len(value)-1:]
}

func maskListenPassword(value string) string {
	if value == "" {
		return ""
	}
	at := strings.LastIndex(value, "@")
	if at == -1 {
		return value
	}
	auth := value[:at]
	host := value[at+1:]
	colon := strings.Index(auth, ":")
	if colon == -1 {
		return value
	}
	return auth[:colon+1] + "******@" + host
}
