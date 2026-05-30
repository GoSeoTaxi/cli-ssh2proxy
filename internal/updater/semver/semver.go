package semver

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type Version struct {
	Major uint64
	Minor uint64
	Patch uint64
	Pre   []identifier
	Build string
}

type identifier struct {
	raw     string
	numeric bool
	value   uint64
}

func Parse(tag string) (Version, error) {
	s := strings.TrimSpace(tag)
	if s == "" {
		return Version{}, errors.New("empty version")
	}
	if strings.HasPrefix(s, "v") || strings.HasPrefix(s, "V") {
		s = s[1:]
	}

	build := ""
	if idx := strings.IndexByte(s, '+'); idx >= 0 {
		build = s[idx+1:]
		s = s[:idx]
		if build == "" {
			return Version{}, fmt.Errorf("invalid version %q: empty build metadata", tag)
		}
	}

	pre := ""
	if idx := strings.IndexByte(s, '-'); idx >= 0 {
		pre = s[idx+1:]
		s = s[:idx]
		if pre == "" {
			return Version{}, fmt.Errorf("invalid version %q: empty prerelease", tag)
		}
	}

	core := strings.Split(s, ".")
	if len(core) != 3 {
		return Version{}, fmt.Errorf("invalid version %q: expected major.minor.patch", tag)
	}

	major, err := parseUint(core[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major in %q: %w", tag, err)
	}
	minor, err := parseUint(core[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor in %q: %w", tag, err)
	}
	patch, err := parseUint(core[2])
	if err != nil {
		return Version{}, fmt.Errorf("invalid patch in %q: %w", tag, err)
	}

	var preParts []identifier
	if pre != "" {
		for _, part := range strings.Split(pre, ".") {
			if part == "" {
				return Version{}, fmt.Errorf("invalid prerelease in %q: empty identifier", tag)
			}
			if !isAlphaNumHyphen(part) {
				return Version{}, fmt.Errorf("invalid prerelease in %q: %q", tag, part)
			}
			id := identifier{raw: part}
			if isNumeric(part) {
				if len(part) > 1 && part[0] == '0' {
					return Version{}, fmt.Errorf("invalid prerelease in %q: numeric identifier %q has leading zero", tag, part)
				}
				val, err := strconv.ParseUint(part, 10, 64)
				if err != nil {
					return Version{}, fmt.Errorf("invalid prerelease in %q: %w", tag, err)
				}
				id.numeric = true
				id.value = val
			}
			preParts = append(preParts, id)
		}
	}

	if build != "" {
		for _, part := range strings.Split(build, ".") {
			if part == "" || !isAlphaNumHyphen(part) {
				return Version{}, fmt.Errorf("invalid build metadata in %q: %q", tag, part)
			}
		}
	}

	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Pre:   preParts,
		Build: build,
	}, nil
}

func Compare(a, b Version) int {
	if c := cmpUint(a.Major, b.Major); c != 0 {
		return c
	}
	if c := cmpUint(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := cmpUint(a.Patch, b.Patch); c != 0 {
		return c
	}

	aPre := len(a.Pre) > 0
	bPre := len(b.Pre) > 0
	if !aPre && !bPre {
		return 0
	}
	if !aPre {
		return 1
	}
	if !bPre {
		return -1
	}

	n := len(a.Pre)
	if len(b.Pre) < n {
		n = len(b.Pre)
	}
	for i := 0; i < n; i++ {
		ai := a.Pre[i]
		bi := b.Pre[i]
		if ai.numeric && bi.numeric {
			if c := cmpUint(ai.value, bi.value); c != 0 {
				return c
			}
			continue
		}
		if ai.numeric && !bi.numeric {
			return -1
		}
		if !ai.numeric && bi.numeric {
			return 1
		}
		if ai.raw < bi.raw {
			return -1
		}
		if ai.raw > bi.raw {
			return 1
		}
	}
	switch {
	case len(a.Pre) < len(b.Pre):
		return -1
	case len(a.Pre) > len(b.Pre):
		return 1
	default:
		return 0
	}
}

func (v Version) String() string {
	out := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Pre) > 0 {
		parts := make([]string, 0, len(v.Pre))
		for _, p := range v.Pre {
			parts = append(parts, p.raw)
		}
		out += "-" + strings.Join(parts, ".")
	}
	if v.Build != "" {
		out += "+" + v.Build
	}
	return out
}

func parseUint(raw string) (uint64, error) {
	if raw == "" {
		return 0, errors.New("empty numeric component")
	}
	if !isNumeric(raw) {
		return 0, fmt.Errorf("non-numeric value %q", raw)
	}
	if len(raw) > 1 && raw[0] == '0' {
		return 0, fmt.Errorf("value %q has leading zero", raw)
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func isNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isAlphaNumHyphen(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			continue
		}
		return false
	}
	return true
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
