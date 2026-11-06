package app

import (
	"path/filepath"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/store"
)

func (o *operation) bindCode(p Object) Object {
	record := o.local(str(p["project_id"]))
	path, repository, kind, topic := str(p["path"]), str(p["repository"]), str(p["kind"]), str(p["topic"])
	branch := p["branch"]
	clearBranch, clearRepo, clearReview := truth(p["clear_branch"]), truth(p["clear_repository"]), truth(p["clear_review"])
	require(path != "" || repository != "" || kind != "" || topic != "" || clearRepo, "Supply a path, source, kind or Topic")
	require(!(repository != "" && clearRepo), "Choose repository or clear-repository")
	require(!(truth(branch) && clearBranch) && (topic != "" || !(truth(branch) || clearBranch)), "Branch changes require one Topic")
	cfg := loadConfig(str(record["path"]), true)
	binding := object(object(cfg["repositories"], "repositories")["code"], "code")
	code := ""
	if path != "" {
		value, e := fsutil.RealPath(path)
		must(e)
		code = value
		o.available(code, str(record["id"]))
		require(!fsutil.Overlap(str(record["path"]), code), "Code and management Store must be separate directory trees")
		require(!fsutil.Exists(code) || fsutil.IsDir(code), "Code path must be a directory")
	}
	var localSource any
	if repository != "" {
		codeURL(repository)
		portable := func() (result string, err error) { defer guard(&err); return portableURL(repository), nil }
		value, e := portable()
		if e != nil {
			localSource = repository
			binding["url"] = nil
		} else {
			binding["url"] = value
		}
	} else if clearRepo {
		binding["url"] = nil
	}
	if kind != "" {
		binding["kind"] = kind
	} else if repository != "" {
		binding["kind"] = "git"
	}
	branches := object(binding["topic_branches"], "topic_branches")
	reviews := object(binding["topic_reviews"], "topic_reviews")
	if topic != "" {
		require(has(object(readState(str(record["path"]))["topics"], "topics"), topic), "Unknown Topic")
		if clearBranch {
			delete(branches, topic)
		} else {
			require(branch != nil || truth(p["review_url"]) || clearReview, "Supply an explicit branch or review association; local Git is not inspected")
			if truth(branch) {
				branches[topic] = branchName(branch)
			}
		}
	}
	require(!(clearReview && (truth(p["review_url"]) || truth(p["target_branch"]))), "Choose review or clear-review")
	require(!(truth(p["review_url"]) || truth(p["target_branch"]) || clearReview) || topic != "", "Review changes require a Topic")
	if clearReview {
		delete(reviews, topic)
	} else if truth(p["review_url"]) || truth(p["target_branch"]) {
		require(truth(p["review_url"]) && truth(p["target_branch"]), "Supply both review-url and target-branch")
		reviews[topic] = Object{"url": reviewURL(p["review_url"]), "target_branch": branchName(p["target_branch"])}
	}
	s, e := store.New(str(record["path"]))
	must(e)
	must(s.WithLock(func() (err error) {
		defer guard(&err)
		contract(o.ctx, "project.save", []any{record["path"], cfg}, func() any { saveConfig(str(record["path"]), cfg); return nil })
		return nil
	}))
	if code != "" {
		record["code_paths"] = []any{filepath.Clean(code)}
	}
	if repository != "" || clearRepo {
		record["code_source"] = localSource
	}
	delete(record, "code_status")
	delete(record, "code_error")
	result := copyObject(record)
	result["repositories"] = cfg["repositories"]
	result["next"] = "Association recorded; context/doctor do not inspect code. Use explicit remote observation if needed"
	return result
}
