package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	KeyPath string

	Login    string
	Password string
	Server   string
	Port     string

	SocksL string
	HTTPL  string
	DNSv6  bool

	SocksUser  string
	SocksPass  string
	HTTPUser   string
	HTTPPass   string
	SocksProxy string

	UseTUN bool

	TimeOutMonitorIntSec int64
	TimeOutMonitor       time.Duration
	Debug                bool
	DNSProvider          string
	DNSServers           []string
	SSHMaxChannels       int64
	SSHProbeMaxChannels  bool
	MaxClientConns       int64
	HandshakeRPS         int64
	HandshakeBurst       int64
	HandshakeIPTTLSec    int64
	AutoUpdate           bool
	AutoUpdateAllowPre   bool
	AutoUpdateInterval   time.Duration
	AutoUpdateTimeout    time.Duration

	UDPGWRemote string
	UDPGWLocal  string
}

const defaultCapacityLimit int64 = 512

func Load() (*Config, error) {
	_ = godotenv.Load()

	useTUN, err := getEnvBool("USE_TUN", false)
	if err != nil {
		return nil, err
	}
	dnsv6, err := getEnvBool("DNS_IPV6", false)
	if err != nil {
		return nil, err
	}
	timeOutMonitorIntSec, err := getEnvInt("TIME_OUT_MONITOR_INT_SEC", 60)
	if err != nil {
		return nil, err
	}
	debug, err := getEnvBool("DEBUG", false)
	if err != nil {
		return nil, err
	}
	sshMaxChannels, sshMaxChannelsExplicit, err := getOptionalEnvInt("SSH_MAX_CHANNELS")
	if err != nil {
		return nil, err
	}
	if sshMaxChannelsExplicit && sshMaxChannels < 0 {
		return nil, fmt.Errorf("invalid SSH_MAX_CHANNELS: must be >= 0")
	}
	sshProbeMaxChannels, err := getEnvBool("SSH_PROBE_MAX_CHANNELS", false)
	if err != nil {
		return nil, err
	}
	maxClientConns, maxClientConnsExplicit, err := getOptionalEnvInt("MAX_CLIENT_CONNS")
	if err != nil {
		return nil, err
	}
	if maxClientConnsExplicit && maxClientConns < 0 {
		return nil, fmt.Errorf("invalid MAX_CLIENT_CONNS: must be >= 0")
	}

	capacityLimit := defaultCapacityLimit
	switch {
	case maxClientConnsExplicit:
		capacityLimit = maxClientConns
		if sshMaxChannelsExplicit && sshMaxChannels != maxClientConns {
			logConfigWarning("MAX_CLIENT_CONNS=%d overrides SSH_MAX_CHANNELS=%d; using %d for both limits", maxClientConns, sshMaxChannels, maxClientConns)
		}
	case sshMaxChannelsExplicit:
		capacityLimit = sshMaxChannels
		logConfigWarning("SSH_MAX_CHANNELS is deprecated, use MAX_CLIENT_CONNS; using %d for both limits", sshMaxChannels)
	}
	handshakeRPS, err := getEnvInt("HANDSHAKE_RPS", 100)
	if err != nil {
		return nil, err
	}
	if handshakeRPS < 0 {
		return nil, fmt.Errorf("invalid HANDSHAKE_RPS: must be >= 0")
	}
	handshakeBurst, err := getEnvInt("HANDSHAKE_BURST", 200)
	if err != nil {
		return nil, err
	}
	if handshakeBurst < 0 {
		return nil, fmt.Errorf("invalid HANDSHAKE_BURST: must be >= 0")
	}
	handshakeIPTTLSec, err := getEnvInt("HANDSHAKE_IP_TTL_SEC", 300)
	if err != nil {
		return nil, err
	}
	if handshakeIPTTLSec < 0 {
		return nil, fmt.Errorf("invalid HANDSHAKE_IP_TTL_SEC: must be >= 0")
	}
	autoUpdate, err := getEnvBool("AUTO_UPDATE", false)
	if err != nil {
		return nil, err
	}
	autoUpdateAllowPre, err := getEnvBool("AUTO_UPDATE_ALLOW_PRERELEASE", false)
	if err != nil {
		return nil, err
	}
	autoUpdateCheckIntervalSec, err := getEnvInt("AUTO_UPDATE_CHECK_INTERVAL_SEC", 0)
	if err != nil {
		return nil, err
	}
	if autoUpdateCheckIntervalSec < 0 {
		return nil, fmt.Errorf("invalid AUTO_UPDATE_CHECK_INTERVAL_SEC: must be >= 0")
	}
	autoUpdateTimeoutSec, err := getEnvInt("AUTO_UPDATE_TIMEOUT_SEC", 15)
	if err != nil {
		return nil, err
	}
	if autoUpdateTimeoutSec <= 0 {
		return nil, fmt.Errorf("invalid AUTO_UPDATE_TIMEOUT_SEC: must be > 0")
	}

	cfg := &Config{

		KeyPath: getEnv("SSH_KEY", ""),
		SocksL:  getEnv("SOCKS_LSN", ""),
		HTTPL:   getEnv("HTTP_LSN", ""),

		UseTUN: useTUN,

		Login:    getEnv("LOGIN", ""),
		Password: getEnv("PASSWORD", ""),
		Server:   getEnv("SERVER", ""),
		Port:     getEnv("PORT", ""),

		DNSv6: dnsv6,

		TimeOutMonitorIntSec: timeOutMonitorIntSec,
		Debug:                debug,
		DNSProvider:          getEnv("DNS_PROVIDER", ""),
		SSHMaxChannels:       capacityLimit,
		SSHProbeMaxChannels:  sshProbeMaxChannels,
		MaxClientConns:       capacityLimit,
		HandshakeRPS:         handshakeRPS,
		HandshakeBurst:       handshakeBurst,
		HandshakeIPTTLSec:    handshakeIPTTLSec,
		AutoUpdate:           autoUpdate,
		AutoUpdateAllowPre:   autoUpdateAllowPre,
		AutoUpdateInterval:   time.Duration(autoUpdateCheckIntervalSec) * time.Second,
		AutoUpdateTimeout:    time.Duration(autoUpdateTimeoutSec) * time.Second,

		UDPGWRemote: getEnv("UDPGW_REMOTE", "127.0.0.1:7300"),
		UDPGWLocal:  getEnv("UDPGW_LOCAL", "127.0.0.1:7300"),
	}
	if cfg.DNSProvider == "" {
		cfg.DNSProvider = getEnv("DNS_Провайдер", "")
	}
	if cfg.DNSProvider == "" {
		cfg.DNSProvider = getEnv("DNS_ПРОВАЙДЕР", "")
	}
	if cfg.DNSProvider == "" {
		cfg.DNSProvider = "DNS_QUERY"
	}

	flag.StringVar(&cfg.Login, "login", cfg.Login, "Login")
	flag.StringVar(&cfg.Password, "password", cfg.Password, "Password")
	flag.StringVar(&cfg.Server, "server", cfg.Server, "Server")
	flag.StringVar(&cfg.Port, "port", cfg.Port, "Port")
	flag.StringVar(&cfg.KeyPath, "key", cfg.KeyPath, "path to private key")

	flag.StringVar(&cfg.SocksL, "socks", cfg.SocksL, "SOCKS5 listen addr")
	flag.StringVar(&cfg.HTTPL, "http", cfg.HTTPL, "HTTP  listen addr")

	flag.BoolVar(&cfg.UseTUN, "tun", cfg.UseTUN, "Use TUN")
	flag.Int64Var(&cfg.TimeOutMonitorIntSec, "timeout-monitor-int-sec", cfg.TimeOutMonitorIntSec, "Timeout monitor interval in seconds")

	flag.StringVar(&cfg.UDPGWRemote, "udpgw-remote", cfg.UDPGWRemote, "Remote UDPGW addr")
	flag.StringVar(&cfg.UDPGWLocal, "udpgw-local", cfg.UDPGWLocal, "Local bind for UDPGW forward")

	flag.BoolVar(&cfg.DNSv6, "dnsv6", cfg.DNSv6, "Resolve AAAA records too")

	flag.StringVar(&cfg.DNSProvider, "dns-provider", cfg.DNSProvider, "DNS provider: DOH_GOOGLE, DOH_CF, DNS_QUERY, DIRECT")
	flag.Int64Var(&cfg.SSHMaxChannels, "ssh-max-channels", cfg.SSHMaxChannels, "Deprecated: use --max-client-conns (shared capacity limit, 0 disables limit)")
	flag.BoolVar(&cfg.SSHProbeMaxChannels, "ssh-probe-max-channels", cfg.SSHProbeMaxChannels, "Run aggressive max-channels probe (diagnostic only)")
	flag.Int64Var(&cfg.MaxClientConns, "max-client-conns", cfg.MaxClientConns, "Shared capacity limit for client admission and SSH channels (0 disables limit)")
	flag.Int64Var(&cfg.HandshakeRPS, "handshake-rps", cfg.HandshakeRPS, "Per-IP handshake token refill rate per second for HTTP/SOCKS (0 disables)")
	flag.Int64Var(&cfg.HandshakeBurst, "handshake-burst", cfg.HandshakeBurst, "Per-IP handshake token bucket size for HTTP/SOCKS")
	flag.Int64Var(&cfg.HandshakeIPTTLSec, "handshake-ip-ttl-sec", cfg.HandshakeIPTTLSec, "Idle TTL in seconds for per-IP handshake buckets")
	flag.BoolVar(&cfg.AutoUpdate, "auto-update", cfg.AutoUpdate, "Enable startup auto-update checks and installation")
	flag.BoolVar(&cfg.AutoUpdateAllowPre, "auto-update-allow-prerelease", cfg.AutoUpdateAllowPre, "Allow prerelease versions when checking for updates")
	flag.DurationVar(&cfg.AutoUpdateInterval, "auto-update-check-interval", cfg.AutoUpdateInterval, "Background auto-update check interval (0 = only at startup)")
	flag.DurationVar(&cfg.AutoUpdateTimeout, "auto-update-timeout", cfg.AutoUpdateTimeout, "Auto-update HTTP timeout")

	flag.BoolVar(&cfg.Debug, "debug", cfg.Debug, "Debug")
	flag.Parse()

	maxClientConnsFlagExplicit := false
	sshMaxChannelsFlagExplicit := false
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "max-client-conns":
			maxClientConnsFlagExplicit = true
		case "ssh-max-channels":
			sshMaxChannelsFlagExplicit = true
		}
	})

	if cfg.SSHMaxChannels < 0 {
		return nil, fmt.Errorf("ssh-max-channels must be >= 0")
	}
	if cfg.MaxClientConns < 0 {
		return nil, fmt.Errorf("max-client-conns must be >= 0")
	}
	if cfg.HandshakeRPS < 0 {
		return nil, fmt.Errorf("handshake-rps must be >= 0")
	}
	if cfg.HandshakeBurst < 0 {
		return nil, fmt.Errorf("handshake-burst must be >= 0")
	}
	if cfg.HandshakeIPTTLSec < 0 {
		return nil, fmt.Errorf("handshake-ip-ttl-sec must be >= 0")
	}
	if cfg.AutoUpdateInterval < 0 {
		return nil, fmt.Errorf("auto-update-check-interval must be >= 0")
	}
	if cfg.AutoUpdateTimeout <= 0 {
		return nil, fmt.Errorf("auto-update-timeout must be > 0")
	}

	resolvedCapacity := cfg.MaxClientConns
	switch {
	case maxClientConnsFlagExplicit:
		resolvedCapacity = cfg.MaxClientConns
		if sshMaxChannelsFlagExplicit && cfg.SSHMaxChannels != cfg.MaxClientConns {
			logConfigWarning("--max-client-conns=%d overrides --ssh-max-channels=%d; using %d for both limits", cfg.MaxClientConns, cfg.SSHMaxChannels, cfg.MaxClientConns)
		}
	case sshMaxChannelsFlagExplicit:
		resolvedCapacity = cfg.SSHMaxChannels
		logConfigWarning("--ssh-max-channels is deprecated, use --max-client-conns; using %d for both limits", cfg.SSHMaxChannels)
	}
	cfg.MaxClientConns = resolvedCapacity
	cfg.SSHMaxChannels = resolvedCapacity

	cfg.SocksL, cfg.SocksUser, cfg.SocksPass, err = parseListenAddr(cfg.SocksL, "SOCKS_LSN")
	if err != nil {
		return nil, err
	}
	cfg.HTTPL, cfg.HTTPUser, cfg.HTTPPass, err = parseListenAddr(cfg.HTTPL, "HTTP_LSN")
	if err != nil {
		return nil, err
	}
	cfg.SocksProxy = buildProxyAddr(cfg.SocksL, cfg.SocksUser, cfg.SocksPass)

	if err := checkSSHConfig(cfg); err != nil {
		return nil, err
	}

	if err := checkProxyConfig(cfg); err != nil {
		return nil, err
	}

	cfg.TimeOutMonitor = time.Duration(cfg.TimeOutMonitorIntSec) * time.Second

	cfg.DNSServers, err = buildDNSServers(cfg.DNSProvider)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func checkSSHConfig(cfg *Config) error {
	if cfg.Login == "" || cfg.Server == "" || cfg.Port == "" {
		return fmt.Errorf("need LOGIN, SERVER and PORT to connect via SSH")
	}
	if !hasAuthMethod(cfg) {
		return fmt.Errorf("need at least one SSH auth method: PASSWORD, SSH_KEY or SSH_AUTH_SOCK")
	}
	return nil
}

func hasAuthMethod(cfg *Config) bool {
	return cfg.Password != "" || cfg.KeyPath != "" || strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK")) != ""
}

func checkProxyConfig(cfg *Config) error {
	if cfg.SocksL == "" && cfg.HTTPL == "" {
		return fmt.Errorf("need at least one of SOCKS or HTTP listen addresses")
	}
	return nil
}

func getEnvInt(k string, def int64) (int64, error) {
	if v := os.Getenv(k); v != "" {
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid %s: %v", k, err)
		}
		return i, nil
	}
	return def, nil
}

func getOptionalEnvInt(k string) (int64, bool, error) {
	raw, ok := os.LookupEnv(k)
	if !ok || strings.TrimSpace(raw) == "" {
		return 0, false, nil
	}
	i, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid %s: %v", k, err)
	}
	return i, true, nil
}

func logConfigWarning(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "config warning: "+format+"\n", args...)
}

func getEnvBool(k string, def bool) (bool, error) {
	if v := os.Getenv(k); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("invalid %s: %q", k, v)
		}
		return b, nil
	}
	return def, nil
}

func buildDNSServers(provider string) ([]string, error) {
	switch strings.ToUpper(strings.TrimSpace(provider)) {
	case "DOH_GOOGLE":
		// DoH endpoint Google Public DNS
		return []string{"https://dns.google/resolve"}, nil
	case "DOH_CF":
		// Cloudflare DoH endpoints
		return []string{
			"https://cloudflare-dns.com/dns-query",
			"https://one.one.one.one/dns-query",
		}, nil
	case "DNS_QUERY":
		return []string{
			// Cloudflare
			"1.1.1.1:53",
			"1.0.0.1:53",

			// Google
			"8.8.8.8:53",
			"8.8.4.4:53",

			// Quad9
			"9.9.9.9:53",
			"149.112.112.112:53",

			// OpenDNS
			"208.67.222.222:53",
			"208.67.220.220:53",
		}, nil
	case "DIRECT":
		return nil, nil
	default:
		return nil, fmt.Errorf("invalid DNS_PROVIDER: %s", provider)
	}
}
