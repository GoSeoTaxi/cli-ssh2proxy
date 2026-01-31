package config

import (
	"fmt"
	"strings"
)

func parseListenAddr(value, name string) (listen, user, pass string, err error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", "", "", nil
	}

	authPart := ""
	hostPart := raw
	if at := strings.LastIndex(raw, "@"); at != -1 {
		authPart = raw[:at]
		hostPart = raw[at+1:]
	}

	if authPart != "" {
		user, pass, err = splitUserPass(authPart)
		if err != nil {
			return "", "", "", fmt.Errorf("%s: %w", name, err)
		}
	}

	hostPart = strings.TrimSpace(hostPart)
	if hostPart == "" {
		return "", "", "", fmt.Errorf("%s: listen address is empty", name)
	}

	return normalizeListenAddr(hostPart), user, pass, nil
}

func splitUserPass(auth string) (string, string, error) {
	parts := strings.SplitN(auth, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid auth format, want user:pass")
	}
	if parts[0] == "" {
		return "", "", fmt.Errorf("empty user in auth")
	}
	return parts[0], parts[1], nil
}

func normalizeListenAddr(addr string) string {
	if addr == "" {
		return ""
	}
	if isDigits(addr) {
		return ":" + addr
	}
	return addr
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func buildProxyAddr(listen, user, pass string) string {
	if listen == "" {
		return ""
	}
	if user == "" && pass == "" {
		return listen
	}
	return fmt.Sprintf("%s:%s@%s", user, pass, listen)
}
