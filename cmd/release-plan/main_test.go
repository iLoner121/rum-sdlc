package main

import "testing"

func TestRepositoryReleaseImpact(t *testing.T) {
	for _, c := range []struct {
		msg   string
		paths []string
		want  int
	}{{"docs: revise skill", []string{"internal/assets/resources/project-skill/SKILL.md"}, 1}, {"docs: readme", []string{"README.md"}, 0}, {"feat: update", nil, 2}, {"feat(app)!: incompatible", nil, 3}, {"chore(main): release 1.0.0", []string{"internal/release/version.go"}, 0}, {"refactor: change\n\nBREAKING CHANGE: behavior", nil, 3}} {
		if got := impact(c.msg, c.paths); got != c.want {
			t.Fatal(c, got)
		}
	}
}
