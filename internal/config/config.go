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

	UDPGWRemote string
	UDPGWLocal  string
}

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
	sshMaxChannels, err := getEnvInt("SSH_MAX_CHANNELS", 64)
	if err != nil {
		return nil, err
	}
	if sshMaxChannels < 0 {
		return nil, fmt.Errorf("invalid SSH_MAX_CHANNELS: must be >= 0")
	}
	sshProbeMaxChannels, err := getEnvBool("SSH_PROBE_MAX_CHANNELS", false)
	if err != nil {
		return nil, err
	}
	maxClientConns := int64(0)
	maxClientConnsExplicit := false
	if raw, ok := os.LookupEnv("MAX_CLIENT_CONNS"); ok && strings.TrimSpace(raw) != "" {
		maxClientConns, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid MAX_CLIENT_CONNS: %v", err)
		}
		maxClientConnsExplicit = true
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
		SSHMaxChannels:       sshMaxChannels,
		SSHProbeMaxChannels:  sshProbeMaxChannels,
		MaxClientConns:       maxClientConns,
		HandshakeRPS:         handshakeRPS,
		HandshakeBurst:       handshakeBurst,
		HandshakeIPTTLSec:    handshakeIPTTLSec,

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
	flag.Int64Var(&cfg.SSHMaxChannels, "ssh-max-channels", cfg.SSHMaxChannels, "Max simultaneously opened SSH channels (0 disables limit)")
	flag.BoolVar(&cfg.SSHProbeMaxChannels, "ssh-probe-max-channels", cfg.SSHProbeMaxChannels, "Run aggressive max-channels probe (diagnostic only)")
	flag.Int64Var(&cfg.MaxClientConns, "max-client-conns", cfg.MaxClientConns, "Max simultaneously accepted client connections across HTTP/SOCKS/TCP listeners (0 disables limit)")
	flag.Int64Var(&cfg.HandshakeRPS, "handshake-rps", cfg.HandshakeRPS, "Per-IP handshake token refill rate per second for HTTP/SOCKS (0 disables)")
	flag.Int64Var(&cfg.HandshakeBurst, "handshake-burst", cfg.HandshakeBurst, "Per-IP handshake token bucket size for HTTP/SOCKS")
	flag.Int64Var(&cfg.HandshakeIPTTLSec, "handshake-ip-ttl-sec", cfg.HandshakeIPTTLSec, "Idle TTL in seconds for per-IP handshake buckets")

	flag.BoolVar(&cfg.Debug, "debug", cfg.Debug, "Debug")
	flag.Parse()

	flag.Visit(func(f *flag.Flag) {
		if f.Name == "max-client-conns" {
			maxClientConnsExplicit = true
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
	if !maxClientConnsExplicit {
		cfg.MaxClientConns = cfg.SSHMaxChannels
	}

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
