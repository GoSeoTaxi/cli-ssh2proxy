package config

import (
	"flag"
	"log"
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

	UDPGWRemote string
	UDPGWLocal  string
}

func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{

		KeyPath: getEnv("SSH_KEY", ""),
		SocksL:  getEnv("SOCKS_LSN", ""),
		HTTPL:   getEnv("HTTP_LSN", ""),

		UseTUN: getEnv("USE_TUN", "false") == "true",

		Login:    getEnv("LOGIN", ""),
		Password: getEnv("PASSWORD", ""),
		Server:   getEnv("SERVER", ""),
		Port:     getEnv("PORT", ""),

		DNSv6: getEnv("DNS_IPV6", "false") == "true",

		TimeOutMonitorIntSec: getEnvInt("TIME_OUT_MONITOR_INT_SEC", 60),
		Debug:                getEnv("DEBUG", "false") == "true",
		DNSProvider:          getEnv("DNS_PROVIDER", ""),

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

	flag.BoolVar(&cfg.Debug, "debug", cfg.Debug, "Debug")
	flag.Parse()

	var err error
	cfg.SocksL, cfg.SocksUser, cfg.SocksPass, err = parseListenAddr(cfg.SocksL, "SOCKS_LSN")
	if err != nil {
		log.Fatal(err)
	}
	cfg.HTTPL, cfg.HTTPUser, cfg.HTTPPass, err = parseListenAddr(cfg.HTTPL, "HTTP_LSN")
	if err != nil {
		log.Fatal(err)
	}
	cfg.SocksProxy = buildProxyAddr(cfg.SocksL, cfg.SocksUser, cfg.SocksPass)

	checkSSHConfig(cfg)

	checkProxyConfig(cfg)

	cfg.TimeOutMonitor = time.Duration(cfg.TimeOutMonitorIntSec) * time.Second

	cfg.DNSServers = buildDNSServers(cfg.DNSProvider)

	return cfg
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func checkSSHConfig(cfg *Config) {
	if cfg.Login == "" || (cfg.Password == "" && cfg.KeyPath == "") || cfg.Server == "" || cfg.Port == "" {
		log.Fatal("Need credentials to connect use SSH")
	}
}

func checkProxyConfig(cfg *Config) {
	if cfg.SocksL == "" && cfg.HTTPL == "" {
		log.Fatal("Need at least one of SOCKS or HTTP listen addresses")
	}
}

func getEnvInt(k string, def int64) int64 {
	if v := os.Getenv(k); v != "" {
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			log.Fatalf("invalid %s: %v", k, err)
		}
		return i
	}
	return def
}

func buildDNSServers(provider string) []string {
	switch strings.ToUpper(strings.TrimSpace(provider)) {
	case "DOH_GOOGLE":
		return []string{"https://dns.google/resolve"}
	case "DOH_CF":
		return []string{"https://cloudflare-dns.com/dns-query"}
	case "DNS_QUERY":
		return []string{
			"1.1.1.1:53",
			"8.8.8.8:53",
			"9.9.9.9:53",
		}
	case "DIRECT":
		return nil
	default:
		log.Fatalf("invalid DNS_PROVIDER: %s", provider)
		return nil
	}
}
