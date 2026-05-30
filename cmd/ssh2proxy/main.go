package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/buildinfo"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/config"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/forward"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/limits"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/logger"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/metrics"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/proxy"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/sshclient"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/tun"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/updater"
)

const (
	keepAliveInterval     = 1 * time.Second
	sleepToReconnect      = 5 * time.Second
	timeCloser            = 2 * time.Second
	timeOutIdleConnection = 30 * time.Second
)

var (
	checkAndUpdateFn = updater.CheckAndUpdate
	restartProcessFn = restartCurrentProcess
)

func applyRuntimeLimits() {
	debug.SetGCPercent(-1)          //  GOGC=off
	debug.SetMemoryLimit(125 << 20) // 125 MiB
}

func main() {

	applyRuntimeLimits()

	if hasVersionFlag(os.Args[1:]) {
		fmt.Println(buildinfo.String())
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load error: %v\n", err)
		os.Exit(1)
	}
	logger.Init(cfg.Debug)

	if err := run(cfg); err != nil {
		zap.L().Error("shutdown with error", zap.Error(err))
		os.Exit(1)
	}
}

func run(cfg *config.Config) error {
	restarted, updateErr := maybeAutoUpdate(cfg)
	if updateErr != nil {
		return updateErr
	}
	if restarted {
		return nil
	}

	logDotEnv()

	bootDNS := proxy.NewDNSResolver(cfg.DNSServers, cfg.DNSv6, nil)

	var (
		sshCl *sshclient.Reconnector
		dial  sshclient.DialFunc
		err   error
	)

	if ip := net.ParseIP(cfg.Server); ip != nil {
		zap.L().Debug("bootstrap DNS skipped: server is IP", zap.String("server", ip.String()))
	}

	resolveServer := func(ctx context.Context, host string) (net.IP, error) {
		_, ip, err := bootDNS.ResolveBoot(ctx, host)
		return ip, err
	}
	sshReconnectOpts := sshclient.ReconnectorOptions{
		MaxChannels: cfg.SSHMaxChannels,
		EnableProbe: cfg.SSHProbeMaxChannels,
	}

	for {
		sshCl, dial, err = sshclient.New(
			cfg.Login,
			cfg.Password,
			cfg.Server,
			cfg.Port,
			cfg.KeyPath,
			resolveServer,
			sshReconnectOpts,
		)
		if err == nil {
			break
		}
		zap.L().Info("SSH connect failed", zap.Error(err), zap.Duration("sleep", sleepToReconnect))
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

		return metrics.WrapConnForMetrics(raw, timeOutIdleConnection), nil
	}

	sessionLimiter := limits.NewSessionLimiter(cfg.MaxClientConns)
	effectiveClientLimit := computeEffectiveClientLimit(sessionLimiter.Max(), cfg.SSHMaxChannels)
	handshakeLimiter := limits.NewIPRateLimiter(
		cfg.HandshakeRPS,
		cfg.HandshakeBurst,
		time.Duration(cfg.HandshakeIPTTLSec)*time.Second,
	)
	zap.L().Info("client_admission_limits",
		zap.Int64("max_client_conns", cfg.MaxClientConns),
		zap.Int64("effective_client_limit", effectiveClientLimit),
		zap.Int64("ssh_max_channels", cfg.SSHMaxChannels),
		zap.Duration("slot_wait_timeout", sshclient.SlotWaitTimeout()),
		zap.Int64("handshake_rps", cfg.HandshakeRPS),
		zap.Int64("handshake_burst", cfg.HandshakeBurst),
		zap.Int64("handshake_ip_ttl_sec", cfg.HandshakeIPTTLSec),
	)
	if reason, risky := capacityMismatchRiskReason(cfg.MaxClientConns, cfg.SSHMaxChannels); risky {
		zap.L().Warn("capacity_mismatch_risk",
			zap.String("reason", reason),
			zap.Int64("max_client_conns", cfg.MaxClientConns),
			zap.Int64("ssh_max_channels", cfg.SSHMaxChannels),
			zap.Int64("effective_client_limit", effectiveClientLimit),
			zap.Duration("slot_wait_timeout", sshclient.SlotWaitTimeout()),
		)
	}

	var fwd *forward.TCPForwarder
	fwd, err = forward.StartTCP(cfg.UDPGWLocal, cfg.UDPGWRemote, dialCount, sessionLimiter)
	if err != nil {
		return fmt.Errorf("udpgw forward: %w", err)
	}
	defer fwd.Close()

	var httpSrv *proxy.HTTPServer
	if cfg.HTTPL != "" {
		httpSrv, err = proxy.NewHTTP(cfg.HTTPL, cfg.HTTPUser, cfg.HTTPPass, dialCount, sessionLimiter, handshakeLimiter)
		if err != nil {
			return fmt.Errorf("HTTP: %w", err)
		}
	}

	var socksSrv *proxy.SocksServer
	if cfg.SocksL != "" {
		socksSrv, err = proxy.NewSOCKS(cfg, dialCount, sessionLimiter, handshakeLimiter)
		if err != nil {
			return fmt.Errorf("SOCKS: %w", err)
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
			return fmt.Errorf("tun2socks external: %w", err)
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
	defer signal.Stop(sig)

	httpErrCh := (<-chan error)(nil)
	if httpSrv != nil {
		httpErrCh = httpSrv.Errors()
	}
	socksErrCh := (<-chan error)(nil)
	if socksSrv != nil {
		socksErrCh = socksSrv.Errors()
	}

	var runtimeErr error
	for {
		select {
		case <-sig:
			zap.L().Info("shutting down…", zap.String("reason", "signal"))
			goto shutdown
		case err, ok := <-httpErrCh:
			if !ok {
				httpErrCh = nil
				continue
			}
			runtimeErr = fmt.Errorf("HTTP proxy Serve error: %w", err)
			zap.L().Error("shutting down on runtime error", zap.Error(runtimeErr))
			goto shutdown
		case err, ok := <-socksErrCh:
			if !ok {
				socksErrCh = nil
				continue
			}
			runtimeErr = fmt.Errorf("SOCKS5 proxy Serve error: %w", err)
			zap.L().Error("shutting down on runtime error", zap.Error(runtimeErr))
			goto shutdown
		}
	}

shutdown:
	ctx, cancel := context.WithTimeout(context.Background(), timeCloser)
	defer cancel()

	if httpSrv != nil {
		if err := httpSrv.Shutdown(ctx); err != nil {
			zap.L().Warn("HTTP shutdown error", zap.Error(err))
		}
	}
	if socksSrv != nil {
		if err := socksSrv.Shutdown(ctx); err != nil && !errors.Is(err, net.ErrClosed) {
			zap.L().Warn("SOCKS shutdown error", zap.Error(err))
		}
	}
	if cmdTun != nil && cmdTun.Process != nil {
		_ = cmdTun.Process.Kill()
	}

	return runtimeErr
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

func hasVersionFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--version" || arg == "-version" {
			return true
		}
	}
	return false
}

func maybeAutoUpdate(cfg *config.Config) (bool, error) {
	if !cfg.AutoUpdate {
		return false, nil
	}
	if cfg.AutoUpdateInterval > 0 {
		zap.L().Info("auto-update background interval is configured, startup-only mode is active in this build",
			zap.Duration("auto_update_interval", cfg.AutoUpdateInterval))
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.AutoUpdateTimeout)
	defer cancel()

	args := append([]string(nil), os.Args[1:]...)
	executablePath, err := os.Executable()
	if err != nil {
		zap.L().Warn("auto-update check failed", zap.Error(fmt.Errorf("resolve executable path: %w", err)))
		return false, nil
	}
	executablePath, err = filepath.Abs(executablePath)
	if err != nil {
		zap.L().Warn("auto-update check failed", zap.Error(fmt.Errorf("resolve executable absolute path: %w", err)))
		return false, nil
	}

	result, err := checkAndUpdateFn(ctx, updater.Options{
		Enabled:         cfg.AutoUpdate,
		AllowPrerelease: cfg.AutoUpdateAllowPre,
		Timeout:         cfg.AutoUpdateTimeout,
		CurrentVersion:  buildinfo.Version,
		AppName:         "ssh2proxy",
		Args:            args,
		ProcessID:       os.Getpid(),
		ExecutablePath:  executablePath,
	})
	if err != nil {
		zap.L().Warn("auto-update check failed", zap.Error(err))
		return false, nil
	}
	if !result.Checked {
		zap.L().Info("auto-update skipped", zap.String("current_version", buildinfo.Version))
		return false, nil
	}
	if !result.UpdateAvailable {
		zap.L().Info("auto-update check complete: no update found",
			zap.String("current_version", result.CurrentVersion),
			zap.String("latest_version", result.LatestVersion),
		)
		return false, nil
	}
	if !result.Updated {
		return false, nil
	}

	zap.L().Info("auto-update installed",
		zap.String("from", result.CurrentVersion),
		zap.String("to", result.LatestVersion),
	)

	if result.WindowsHelperStarted {
		zap.L().Info("windows staged update helper started; exiting current process")
		return true, nil
	}

	if err := restartProcessFn(executablePath, args); err != nil {
		return false, fmt.Errorf("restart after update: %w", err)
	}
	return true, nil
}

func restartCurrentProcess(executablePath string, args []string) error {
	exePath := strings.TrimSpace(executablePath)
	if exePath == "" {
		return errors.New("resolve executable for restart: executable path is empty")
	}
	cmd := exec.Command(exePath, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}

func computeEffectiveClientLimit(clientLimit, channelLimit int64) int64 {
	if clientLimit <= 0 && channelLimit <= 0 {
		return 0
	}
	if clientLimit <= 0 {
		return channelLimit
	}
	if channelLimit <= 0 {
		return clientLimit
	}
	if clientLimit < channelLimit {
		return clientLimit
	}
	return channelLimit
}

func capacityMismatchRiskReason(clientLimit, channelLimit int64) (string, bool) {
	if channelLimit <= 0 {
		return "", false
	}
	if clientLimit <= 0 {
		return "max_client_conns is unlimited while ssh_max_channels is limited", true
	}
	if clientLimit > channelLimit {
		return "max_client_conns is greater than ssh_max_channels", true
	}
	return "", false
}
