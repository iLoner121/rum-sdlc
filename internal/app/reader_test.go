package app

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"rumsdlc/internal/core"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/store"
	"strings"
	"testing"
)

func readerFixture(t *testing.T, n int) (*Reader, *store.Store) {
	t.Helper()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	home := filepath.Join(root, "home")
	s, e := store.New(filepath.Join(root, "meta"))
	if e != nil {
		t.Fatal(e)
	}
	var state Object
	step := func(action string, p Object) {
		var e error
		state, _, e = core.Dispatch(state, core.Request(action, p, "2026-11-18", "fixture", "fixture", "fixture"))
		if e != nil {
			t.Fatal(action, e)
		}
	}
	step("project.init", Object{"id": "P-reader", "name": "中文工程"})
	for i := 0; i < n; i++ {
		step("topic.create", Object{"name": fmt.Sprintf("需求 %d", i+1), "slug": "fixture", "type": "feat", "unassigned_reason": "fixture"})
	}
	if e = os.MkdirAll(s.Root, 0700); e != nil {
		t.Fatal(e)
	}
	if e = store.Atomic(filepath.Join(s.Root, "state.json"), state); e != nil {
		t.Fatal(e)
	}
	if e = fsutil.WriteJSON(filepath.Join(home, "registry.json"), Object{"format": "rumsdlc-device/1", "projects": Object{"P-reader": Object{"id": "P-reader", "name": "中文工程", "path": s.Root, "status": "local-only"}}, "repositories": Object{}}); e != nil {
		t.Fatal(e)
	}
	r, e := NewReader(home)
	if e != nil {
		t.Fatal(e)
	}
	return r, s
}
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			t.Fatal(e)
		}
		if d.IsDir() {
			info, _ := d.Info()
			out[path] = "directory/" + info.ModTime().String()
		} else {
			raw, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			info, _ := d.Info()
			out[path] = jsonvalue.HashBytes(raw) + info.ModTime().String()
		}
		return nil
	})
	return out
}
func TestReaderEmptyHomeNeverInitializes(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	home := filepath.Join(root, "absent")
	r, e := NewReader(home)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.ReadProjects(); e != nil {
		t.Fatal(e)
	}
	if _, e = r.ReadDevice(); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(home); !os.IsNotExist(e) {
		t.Fatal("read initialized home", e)
	}
}
func TestReaderFullScopeSearchPaginationAndNoPersistence(t *testing.T) {
	r, s := readerFixture(t, 205)
	before := tree(t, filepath.Dir(s.Root))
	v, e := r.ReadTopics("P-reader", TopicFilter{Query: "需求 205", Limit: 30})
	if e != nil {
		t.Fatal(e)
	}
	d := v.Data.(Object)
	if d["total"] != 1 || d["scope_total"] != 205 || d["rows"].([]any)[0].(Object)["short_id"] != "T205" {
		t.Fatal(d)
	}
	if _, e = r.ReadOverview("P-reader", ""); e != nil {
		t.Fatal(e)
	}
	if _, e = r.ReadTopic("P-reader", "T205", ""); e != nil {
		t.Fatal(e)
	}
	if _, e = r.ReadProjects(); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, tree(t, filepath.Dir(s.Root))) {
		t.Fatal("reading changed persisted files")
	}
	if _, e = r.ReadTopics("P-reader", TopicFilter{Revision: "0"}); e == nil {
		t.Fatal("accepted stale revision")
	}
}
func TestReaderRejectsCrossProjectAndInvalidStatus(t *testing.T) {
	r, s := readerFixture(t, 1)
	db, _ := r.ReadRegistry()
	getObject(db, "projects")["P-wrong"] = Object{"path": s.Root}
	fsutil.WriteJSON(filepath.Join(r.Home, "registry.json"), db)
	v, e := r.ReadProjects()
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range v.Data.(Object)["rows"].([]any) {
		row := v.(Object)
		if row["id"] == "P-wrong" && row["local_available"] != false {
			t.Fatal(row)
		}
	}
	if _, e = r.ReadOverview("P-wrong", ""); e == nil {
		t.Fatal("cross-project identity accepted")
	}
	if _, e = r.ReadTopics("P-reader", TopicFilter{Status: "done"}); e == nil {
		t.Fatal("invalid topic status accepted")
	}
}
func TestReaderDocumentsBoundedAndSafeLinks(t *testing.T) {
	r, s := readerFixture(t, 1)
	info, e := s.Document(store.DocumentOptions{Table: "topics", ID: "T1", Field: "context", LocateOnly: true})
	if e != nil {
		t.Fatal(e)
	}
	path := info["path"].(string)
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte("# 中文\n[设计](DESIGN.md)"), 0600)
	os.WriteFile(filepath.Join(filepath.Dir(path), "DESIGN.md"), []byte("# 设计"), 0600)
	v, e := r.ReadDocument("P-reader", "topics", "T1", "context", "")
	if e != nil || v.Data.(Object)["exists"] != true {
		t.Fatal(v, e)
	}
	if _, ok := v.Data.(Object)["path"]; ok {
		t.Fatal("absolute path leaked")
	}
	linked, e := r.ReadDocumentLink("P-reader", "topics", "T1", "context", "DESIGN.md", "")
	if e != nil || linked.Data.(Object)["content"] != "# 设计" {
		t.Fatal(linked, e)
	}
	resource := linked.Data.(Object)["resource"].(string)
	if _, e = r.ReadDocumentResource("P-reader", resource, "TASK_CONTEXT.md", ""); e != nil {
		t.Fatal(e)
	}
	for _, href := range []string{"../../../../secret.md", "file:///etc/passwd", "javascript:alert(1)", "%2fetc/a.md", "/etc/a.md", "%2F%2Fevil/a.md", "C:/a.md", "C:a.md", "C%3a/a.md", "file%3astream.md", "DESIGN.html", "//evil/a.md", "bad\\file.md", "bad%5cfile.md"} {
		if _, e = r.ReadDocumentLink("P-reader", "topics", "T1", "context", href, ""); e == nil {
			t.Fatal("accepted unsafe link", href)
		}
	}
	for _, resource := range []string{"/etc/a.md", "//server/a.md", "C:/a.md", "C:a.md", "file:stream.md", `bad\file.md`} {
		if _, e = r.ReadDocumentResource("P-reader", resource, "DESIGN.md", ""); e == nil {
			t.Fatal("accepted unsafe resource", resource)
		}
	}
	os.Symlink(filepath.Join(filepath.Dir(s.Root), "secret.md"), filepath.Join(filepath.Dir(path), "link.md"))
	if _, e = r.ReadDocumentLink("P-reader", "topics", "T1", "context", "link.md", ""); e == nil {
		t.Fatal("accepted symlink")
	}
	os.WriteFile(path, []byte(strings.Repeat("x", MaxDocumentBytes+1)), 0600)
	if _, e = r.ReadDocument("P-reader", "topics", "T1", "context", ""); e == nil {
		t.Fatal("oversize accepted")
	}
	os.WriteFile(path, []byte{0xff}, 0600)
	if _, e = r.ReadDocument("P-reader", "topics", "T1", "context", ""); e == nil {
		t.Fatal("invalid UTF8 accepted")
	}
}

func TestReaderIterationUsesFrozenResultsAndPoolOrder(t *testing.T) {
	r, s := readerFixture(t, 3)
	state, e := s.Read(false)
	if e != nil {
		t.Fatal(e)
	}
	step := func(action string, p Object) {
		var e error
		state, _, e = core.Dispatch(state, core.Request(action, p, "2026-11-18", "fixture", "fixture", "fixture"))
		if e != nil {
			t.Fatal(action, e)
		}
	}
	step("workset.create", Object{"name": "历史轮次"})
	step("workset.propose", Object{"id": "ITER-001", "topics": []any{"T2", "T1"}})
	step("workset.activate", Object{"id": "ITER-001"})
	if e = store.Atomic(filepath.Join(s.Root, "state.json"), state); e != nil {
		t.Fatal(e)
	}
	v, e := r.ReadTopics("P-reader", TopicFilter{Scope: "iteration", WorksetID: "ITER-001"})
	if e != nil {
		t.Fatal(e)
	}
	rows := v.Data.(Object)["rows"].([]any)
	if len(rows) != 2 || rows[0].(Object)["short_id"] != "T2" || rows[1].(Object)["short_id"] != "T1" {
		t.Fatal(rows)
	}
	step("topic.close", Object{"id": "T2", "outcome": "cancelled", "summary": "Cancelled", "verification": "Reviewed", "retention": "Keep"})
	step("workset.close", Object{"id": "ITER-001", "outcome": "replanned", "summary": "Replanned", "results": []any{Object{"topic": "T2", "disposition": "cancelled", "note": "Cancelled", "evidence": "fixture"}, Object{"topic": "T1", "disposition": "backlog", "note": "Backlog", "evidence": "fixture"}}})
	if e = store.Atomic(filepath.Join(s.Root, "state.json"), state); e != nil {
		t.Fatal(e)
	}
	v, e = r.ReadTopics("P-reader", TopicFilter{Scope: "iteration", WorksetID: "ITER-001"})
	if e != nil {
		t.Fatal(e)
	}
	rows = v.Data.(Object)["rows"].([]any)
	if v.Basis != "historical-results" || len(rows) != 2 || rows[0].(Object)["disposition"] != "cancelled" || rows[1].(Object)["disposition"] != "backlog" {
		t.Fatal(v)
	}
	if _, e = r.ReadTopics("P-reader", TopicFilter{Scope: "iteration", WorksetID: "ITER-001", Status: "planned"}); e == nil {
		t.Fatal("accepted current filter on frozen results")
	}
	pool, e := r.ReadTopics("P-reader", TopicFilter{Scope: "pool"})
	if e != nil {
		t.Fatal(e)
	}
	if pool.Data.(Object)["scope_total"] != 2 {
		t.Fatal(pool)
	}
}

func TestReaderDraftIterationShowsProposedTopicsInProposalOrder(t *testing.T) {
	r, s := readerFixture(t, 3)
	state, e := s.Read(false)
	if e != nil {
		t.Fatal(e)
	}
	for _, request := range []Object{core.Request("workset.create", Object{"name": "草案"}, "2026-11-18", "fixture", "fixture", "fixture"), core.Request("workset.propose", Object{"id": "ITER-001", "topics": []any{"T3", "T1"}}, "2026-11-18", "fixture", "fixture", "fixture")} {
		state, _, e = core.Dispatch(state, request)
		if e != nil {
			t.Fatal(e)
		}
	}
	if e = store.Atomic(filepath.Join(s.Root, "state.json"), state); e != nil {
		t.Fatal(e)
	}
	planning, e := r.ReadPlanning("P-reader", "worksets", "", TopicFilter{})
	if e != nil {
		t.Fatal(e)
	}
	planned := planning.Data.(Object)["rows"].([]any)[0].(Object)
	if planned["member_count"] != 2 || planned["proposed_count"] != 2 || planned["progress"].(Object)["basis"] != "draft-proposal" {
		t.Fatal(planned)
	}
	topics, e := r.ReadTopics("P-reader", TopicFilter{Scope: "iteration", WorksetID: "ITER-001"})
	if e != nil {
		t.Fatal(e)
	}
	rows := topics.Data.(Object)["rows"].([]any)
	if topics.Basis != "draft-proposal" || len(rows) != 2 || rows[0].(Object)["short_id"] != "T3" || rows[1].(Object)["short_id"] != "T1" {
		t.Fatal(topics)
	}
	if _, e = r.ReadTopics("P-reader", TopicFilter{Scope: "iteration", WorksetID: "ITER-001", Basis: "invalid"}); e == nil {
		t.Fatal("invalid basis accepted")
	}
}
func TestReaderPaginationMaximumOffsetReturnsEmpty(t *testing.T) {
	r, _ := readerFixture(t, 1)
	maximum := int(^uint(0) >> 1)
	topics, e := r.ReadTopics("P-reader", TopicFilter{Offset: maximum, Limit: 30})
	if e != nil {
		t.Fatal(e)
	}
	if len(topics.Data.(Object)["rows"].([]any)) != 0 || topics.Data.(Object)["total"] != 1 {
		t.Fatal(topics)
	}
	planning, e := r.ReadPlanning("P-reader", "milestones", "", TopicFilter{Offset: maximum, Limit: 200})
	if e != nil {
		t.Fatal(e)
	}
	if len(planning.Data.(Object)["rows"].([]any)) != 0 {
		t.Fatal(planning)
	}
}
