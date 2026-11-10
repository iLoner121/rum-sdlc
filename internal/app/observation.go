package app

import (
	"context"
	"regexp"
	"rumsdlc/internal/core"
	"rumsdlc/internal/fsutil"
	"strconv"
	"strings"
	"time"
)

// ReviewReader is separate from metadata Hub so observation never inherits the
// device-selected metadata account or reads a local code repository.
type ReviewReader func(context.Context, string) (Object, error)

func readReview(ctx context.Context, address string) (result Object, err error) {
	defer guard(&err)
	match := regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/pull/([1-9][0-9]*)$`).FindStringSubmatch(address)
	require(match != nil, "Unsupported review provider or URL; use the project skill provider workflow")
	owner, repo, number := match[1], match[2], match[3]
	g := NewGitHub()
	g.Client.Timeout = 12 * time.Second
	v, e := g.api(ctx, "repos/"+owner+"/"+repo+"/pulls/"+number, "GET", nil)
	must(e)
	return validateReview(object(v, "review response"), address, owner+"/"+repo, number), nil
}
func validateReview(value Object, address, repository, number string) Object {
	merged, ok := value["merged"].(bool)
	require(ok, "Invalid review response")
	n, e := strconv.ParseInt(number, 10, 64)
	must(e)
	require(integer(value["number"]) == n && value["html_url"] == address, "Review identity mismatch")
	base, head := object(value["base"], "Missing review refs"), object(value["head"], "Missing review refs")
	require(strings.EqualFold(str(object(base["repo"], "Target repository")["full_name"]), repository), "Target repository mismatch")
	sha := regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	for _, ref := range []Object{base, head} {
		_, ok := ref["ref"].(string)
		require(ok && sha.MatchString(str(ref["sha"])), "Invalid review ref")
	}
	require(oneOf(str(value["state"]), "open closed"), "Unknown review state")
	var mergeCommit any
	if merged {
		_, ok := value["merged_at"].(string)
		require(value["state"] == "closed" && ok && sha.MatchString(str(value["merge_commit_sha"])), "Incomplete merge evidence")
		mergeCommit = value["merge_commit_sha"]
	}
	return Object{"merged": merged, "state": value["state"], "merged_at": value["merged_at"], "merge_commit": mergeCommit, "target_branch": base["ref"], "target_head": base["sha"], "source_branch": head["ref"], "source_head": head["sha"]}
}
func (o *operation) observe(p Object) Object {
	record := o.find(p)
	cfg := loadConfig(str(record["path"]), true)
	state := readState(str(record["path"]))
	report := Object{"observed_at": fsutil.Now(), "blocking": false, "status": "not-requested", "scope": "registered-remote-review", "local_code_inspected": false, "topics": []any{}, "findings": []any{}, "note": "Historical remote merge facts only. No proof of current target content, tests, acceptance or permission to push."}
	topic := str(p["topic"])
	if p["topic"] != nil {
		var err error
		topic, err = core.CanonicalReference(state, topic)
		must(err)
		require(has(object(state["topics"], "topics"), topic), "Unknown Topic")
	}
	if !truth(p["remote"]) {
		report["next"] = "Use --remote --topic ID to query one registered review explicitly"
		return report
	}
	require(topic != "", "Remote observation requires one explicit --topic")
	review := getObject(object(object(cfg["repositories"], "repositories")["code"], "code"), "topic_reviews")[topic]
	if review == nil {
		report["status"] = "unbound"
		report["next"] = "Register --review-url and --target-branch through bind-code"
		return report
	}
	binding := object(review, "review")
	row := Object{"short_id": core.ShortReference(state, topic), "topic_id": topic, "url": binding["url"], "expected_target_branch": binding["target_branch"], "status": "unknown", "observed_at": fsutil.Now()}
	report["topics"] = []any{row}
	reader := o.device.Review
	if reader == nil {
		reader = readReview
	}
	facts, e := reader(o.ctx, str(binding["url"]))
	if e != nil {
		report["status"] = "unknown"
		report["findings"] = []any{Object{"code": "remote-observation-unavailable", "blocking": false, "message": "No reliable result. Check provider support, credentials, network and response shape in the observation process."}}
		return report
	}
	status := "closed-unmerged"
	if !equal(facts["target_branch"], binding["target_branch"]) {
		status = "target-mismatch"
	} else if facts["merged"] == true {
		status = "merged"
	} else if facts["state"] == "open" {
		status = "open"
	}
	row["status"] = status
	row["facts"] = facts
	report["status"] = status
	return report
}
