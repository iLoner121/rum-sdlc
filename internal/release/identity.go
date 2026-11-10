package release

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
)

// Set only by the release build entrypoint. Ordinary go build is never stable.
var Channel, Commit, Dirty, BuiltAt string

type Identity struct {
	Format  string `json:"format"`
	Version string `json:"version"`
	Channel string `json:"channel"`
	Commit  string `json:"commit"`
	Dirty   bool   `json:"dirty"`
	BuiltAt string `json:"built_at,omitempty"`
}

func Current() Identity {
	i := Identity{Format: "rum-build/1", Version: Version, Channel: Channel, Commit: Commit, Dirty: Dirty == "true", BuiltAt: BuiltAt}
	if i.Channel == "" {
		i.Channel = "dev"
	}
	if b, ok := debug.ReadBuildInfo(); ok && i.Commit == "" {
		for _, s := range b.Settings {
			if s.Key == "vcs.revision" {
				i.Commit = s.Value
			}
			if s.Key == "vcs.modified" {
				i.Dirty = s.Value == "true"
			}
		}
	}
	if i.Commit == "" {
		i.Commit = "unknown"
		if i.Channel != "stable" {
			i.Channel = "unknown"
		}
	}
	return i
}
func (i Identity) Display() string {
	if i.Channel == "stable" {
		return i.Version
	}
	c := i.Commit
	if len(c) > 12 {
		c = c[:12]
	}
	suffix := ""
	if i.Dirty {
		suffix = ".dirty"
	}
	return i.Version + "-" + i.Channel + "+g" + c + suffix
}
func (i Identity) Validate() error {
	if _, e := Parse(i.Version); e != nil {
		return e
	}
	if i.Channel != "stable" && i.Channel != "dev" && i.Channel != "unknown" {
		return fmt.Errorf("Invalid build channel")
	}
	if i.Channel == "stable" && (i.Dirty || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(i.Commit)) {
		return fmt.Errorf("Stable build requires a clean, exact source commit")
	}
	return nil
}

type SemVer struct {
	Parts [3]string
	Pre   []string
}

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

func Parse(s string) (SemVer, error) {
	m := versionPattern.FindStringSubmatch(s)
	if m == nil {
		return SemVer{}, fmt.Errorf("Invalid semantic version: %q", s)
	}
	v := SemVer{Parts: [3]string{m[1], m[2], m[3]}}
	if m[4] != "" {
		v.Pre = strings.Split(m[4], ".")
	}
	for _, p := range v.Pre {
		if numeric(p) && len(p) > 1 && p[0] == '0' {
			return SemVer{}, fmt.Errorf("Invalid numeric prerelease identifier")
		}
	}
	return v, nil
}
func numeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}
func number(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}
func Compare(a, b string) (int, error) {
	x, e := Parse(a)
	if e != nil {
		return 0, e
	}
	y, e := Parse(b)
	if e != nil {
		return 0, e
	}
	for n := range x.Parts {
		if c := number(x.Parts[n], y.Parts[n]); c != 0 {
			return c, nil
		}
	}
	if len(x.Pre) == 0 && len(y.Pre) > 0 {
		return 1, nil
	}
	if len(y.Pre) == 0 && len(x.Pre) > 0 {
		return -1, nil
	}
	for n := 0; n < len(x.Pre) && n < len(y.Pre); n++ {
		a, b := x.Pre[n], y.Pre[n]
		c := 0
		switch {
		case numeric(a) && numeric(b):
			c = number(a, b)
		case numeric(a):
			c = -1
		case numeric(b):
			c = 1
		default:
			c = strings.Compare(a, b)
		}
		if c != 0 {
			return c, nil
		}
	}
	if len(x.Pre) < len(y.Pre) {
		return -1, nil
	}
	if len(x.Pre) > len(y.Pre) {
		return 1, nil
	}
	return 0, nil
}
