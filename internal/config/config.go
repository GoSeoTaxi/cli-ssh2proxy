package config

import (
	"flag"
	"log"
	"os"

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

	UseTUN bool

	Debug bool
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

		Debug: getEnv("DEBUG", "false") == "true",
	}

	flag.StringVar(&cfg.Login, "login", cfg.Login, "Login")
	flag.StringVar(&cfg.Password, "password", cfg.Password, "Password")
	flag.StringVar(&cfg.Server, "server", cfg.Server, "Server")
	flag.StringVar(&cfg.Port, "port", cfg.Port, "Port")
	flag.StringVar(&cfg.KeyPath, "key", cfg.KeyPath, "path to private key")

	flag.StringVar(&cfg.SocksL, "socks", cfg.SocksL, "SOCKS5 listen addr")
	flag.StringVar(&cfg.HTTPL, "http", cfg.HTTPL, "HTTP  listen addr")

	flag.BoolVar(&cfg.UseTUN, "tun", cfg.UseTUN, "Use TUN")
	flag.BoolVar(&cfg.Debug, "debug", cfg.Debug, "Debug")
	flag.Parse()

	checkSSHConfig(cfg)

	checkProxyConfig(cfg)

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
		log.Fatal("Don't use both SOCKS and HTTP")
	}
}
