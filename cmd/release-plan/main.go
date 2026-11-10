// release-plan is repository release policy, not a rule installed in projects.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"rumsdlc/internal/release"
	"strconv"
	"strings"
)

func git(args ...string) (string, error) {
	b, e := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(b)), e
}

type Plan struct {
	Base       string   `json:"base"`
	Source     string   `json:"source"`
	Version    string   `json:"version"`
	Releasable bool     `json:"releasable"`
	Changes    []string `json:"changes"`
}

func impact(message string, paths []string) int {
	title := strings.Split(message, "\n")[0]
	if regexp.MustCompile(`^chore(?:\([^)]*\))?: release `).MatchString(title) {
		return 0
	}
	if strings.Contains(message, "BREAKING CHANGE:") || strings.Contains(message, "BREAKING-CHANGE:") || regexp.MustCompile(`^[a-z]+(?:\([^)]*\))?!:`).MatchString(title) {
		return 3
	}
	if strings.HasPrefix(title, "feat:") || strings.HasPrefix(title, "feat(") {
		return 2
	}
	if strings.HasPrefix(title, "fix:") || strings.HasPrefix(title, "fix(") || strings.HasPrefix(title, "perf:") || strings.HasPrefix(title, "perf(") {
		return 1
	}
	for _, p := range paths {
		if strings.HasPrefix(p, "internal/") || strings.HasPrefix(p, "cmd/") || strings.HasPrefix(p, "installer-skill/") || p == "go.mod" {
			return 1
		}
	}
	return 0
}
func plan(base, source string) (Plan, error) {
	p := Plan{Base: base, Source: source, Version: strings.TrimPrefix(base, "v"), Changes: []string{}}
	v, e := release.Parse(p.Version)
	if e != nil {
		return p, e
	}
	if len(v.Pre) > 0 {
		return p, fmt.Errorf("baseline must be stable")
	}
	if _, e = git("merge-base", "--is-ancestor", base, source); e != nil {
		return p, fmt.Errorf("release baseline is not an ancestor")
	}
	commits, e := git("rev-list", "--reverse", base+".."+source)
	if e != nil {
		return p, e
	}
	bump := 0
	for _, sha := range strings.Fields(commits) {
		message, e := git("show", "-s", "--format=%B", sha)
		if e != nil {
			return p, e
		}
		paths, e := git("diff-tree", "--root", "--no-commit-id", "--name-only", "-r", sha)
		if e != nil {
			return p, e
		}
		n := impact(message, strings.Split(paths, "\n"))
		if n > 0 {
			p.Changes = append(p.Changes, sha+" "+strings.Split(message, "\n")[0])
		}
		if n > bump {
			bump = n
		}
	}
	if bump == 0 {
		return p, nil
	}
	numbers := [3]uint64{}
	for i, x := range v.Parts {
		numbers[i], e = strconv.ParseUint(x, 10, 64)
		if e != nil {
			return p, e
		}
	}
	at := 3 - bump
	if numbers[at] == ^uint64(0) {
		return p, fmt.Errorf("version overflow")
	}
	numbers[at]++
	for i := at + 1; i < 3; i++ {
		numbers[i] = 0
	}
	p.Version = fmt.Sprintf("%d.%d.%d", numbers[0], numbers[1], numbers[2])
	p.Releasable = true
	return p, nil
}
func main() {
	base := flag.String("base", "", "latest published stable tag")
	source := flag.String("source", "HEAD", "fixed candidate commit")
	flag.Parse()
	p, e := plan(*base, *source)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(p)
}
