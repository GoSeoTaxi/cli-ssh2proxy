package sshclient

import (
	"errors"
	"net"
	"strings"
)

func RejectIPv6(addr string, allowV6 bool) error {
	if allowV6 {
		return nil
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil
	}

	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if ip != nil && ip.To4() == nil {
		return errors.New("IPv6 target rejected by policy")
	}
	return nil
}
