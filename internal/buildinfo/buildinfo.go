package buildinfo

import (
	"fmt"
	"runtime"
	"strings"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func IsDev() bool {
	return strings.TrimSpace(Version) == "" || strings.TrimSpace(Version) == "dev"
}

func String() string {
	return fmt.Sprintf(
		"ssh2proxy version=%s commit=%s build_date=%s go=%s os=%s arch=%s",
		valueOrDefault(Version, "dev"),
		valueOrDefault(Commit, "unknown"),
		valueOrDefault(BuildDate, "unknown"),
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
	)
}

func valueOrDefault(v, def string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	return v
}
