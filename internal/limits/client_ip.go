package limits

import "net"

func ClientIPFromRemoteAddr(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		if tcpAddr, ok := addr.(*net.TCPAddr); ok && tcpAddr.IP != nil {
			return tcpAddr.IP.String()
		}
		return addr.String()
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return host
}
