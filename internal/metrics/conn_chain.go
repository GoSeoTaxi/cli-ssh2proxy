package metrics

import (
	"net"
	"time"
)

// WrapConnForMetrics applies connection wrappers in a safe order:
// TrackConn must be inside IdleConn so idle watchdog close decrements openConns.
func WrapConnForMetrics(c net.Conn, idle time.Duration) net.Conn {
	return NewIdleConn(&CountConn{Conn: NewTrackConn(c)}, idle)
}
