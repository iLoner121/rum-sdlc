package app

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/store"
	"strings"
	"unicode/utf8"
)

const MaxDocumentBytes = 2 * 1024 * 1024

func documentInfo(s *store.Store, table, id, field string) (Object, error) {
	allowed := map[string]string{"project": "description", "milestones": "description", "worksets": "description", "lines": "description", "topics": "context prd design acceptance description", "tasks": "description"}
	if field == "" {
		field = "description"
	}
	if !oneOf(field, allowed[table]) {
		return nil, NewReadError("invalid", "Unsupported document field")
	}
	d, e := s.Document(store.DocumentOptions{Table: table, ID: id, Field: field, LocateOnly: true})
	if e != nil {
		return nil, NewReadError("path-unavailable", "Document is unavailable")
	}
	delete(d, "metadata")
	d["table"] = table
	d["id"] = id
	d["field"] = field
	return d, nil
}
func (r *Reader) ReadDocument(pid, table, id, field, revision string) (View, error) {
	s, state, e := r.snapshot(pid, revision)
	if e != nil {
		return View{}, e
	}
	d, e := documentInfo(s, table, id, field)
	if e != nil {
		return View{}, e
	}
	path := str(d["path"])
	delete(d, "path")
	resource, _ := filepath.Rel(filepath.Join(s.Root, "documents"), path)
	d["resource"] = filepath.ToSlash(resource)
	d["hash"] = jsonvalue.HashBytes(nil)
	d["content"] = ""
	if d["exists"] == true {
		rel, e := filepath.Rel(s.Root, path)
		if e != nil {
			return View{}, NewReadError("path-unavailable", "Document is unavailable")
		}
		safe, e := store.Safe(s.Root, rel)
		if e != nil {
			return View{}, NewReadError("path-unavailable", "Document is unavailable")
		}
		f, e := os.Open(safe)
		if e != nil {
			return View{}, NewReadError("path-unavailable", "Document is unavailable")
		}
		defer f.Close()
		info, e := f.Stat()
		if e != nil || !info.Mode().IsRegular() {
			return View{}, NewReadError("path-unavailable", "Document is not a regular file")
		}
		raw, e := io.ReadAll(io.LimitReader(f, MaxDocumentBytes+1))
		if e != nil {
			return View{}, NewReadError("path-unavailable", "Document cannot be read")
		}
		if len(raw) > MaxDocumentBytes {
			return View{}, NewReadError("too-large", "Document exceeds the 2 MiB preview limit")
		}
		if !utf8.Valid(raw) {
			return View{}, NewReadError("invalid", "Document is not UTF-8 text")
		}
		d["content"] = string(raw)
		d["hash"] = jsonvalue.HashBytes(raw)
	}
	view := NewView(pid, state["revision"], "document", d)
	d["read_at"] = view.ReadAt
	return view, nil
}

// ReadDocumentLink resolves only Markdown under the Store's documents tree.
// href remains relative to the trusted document location; no caller supplies an
// absolute filesystem path, and raw components reach Safe before normalization.
func (r *Reader) ReadDocumentLink(pid, table, id, field, href, revision string) (View, error) {
	s, state, e := r.snapshot(pid, revision)
	if e != nil {
		return View{}, e
	}
	base, e := documentInfo(s, table, id, field)
	if e != nil {
		return View{}, e
	}
	return r.readDocumentLink(s, state, pid, str(base["path"]), href)
}
func (r *Reader) ReadDocumentResource(pid, resource, href, revision string) (View, error) {
	s, state, e := r.snapshot(pid, revision)
	if e != nil {
		return View{}, e
	}
	if resource == "" || strings.HasPrefix(resource, "/") || strings.ContainsAny(resource, "\\:") || !strings.EqualFold(filepath.Ext(resource), ".md") {
		return View{}, NewReadError("invalid", "Invalid document resource")
	}
	base, e := store.Safe(s.Root, "documents/"+resource)
	if e != nil {
		return View{}, NewReadError("permission", "Invalid document resource")
	}
	rel, e := filepath.Rel(filepath.Join(s.Root, "documents"), base)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return View{}, NewReadError("permission", "Invalid document resource")
	}
	return r.readDocumentLink(s, state, pid, base, href)
}
func (r *Reader) readDocumentLink(s *store.Store, state Object, pid, basePath, href string) (View, error) {
	u, e := url.Parse(href)
	if e != nil || u.IsAbs() || u.Host != "" || u.Opaque != "" || u.RawQuery != "" || strings.Contains(href, "\\") {
		return View{}, NewReadError("invalid", "Only relative Markdown links are supported")
	}
	// URL paths use slash semantics on every OS. filepath.IsAbs alone
	// accepts root-relative paths on Windows; colons also allow drive/ADS names.
	relative := u.Path
	if relative == "" || strings.HasPrefix(relative, "/") || strings.ContainsAny(relative, "\\:") || !strings.EqualFold(filepath.Ext(relative), ".md") {
		return View{}, NewReadError("invalid", "Only relative Markdown links are supported")
	}
	basedir, e := filepath.Rel(s.Root, filepath.Dir(basePath))
	if e != nil {
		return View{}, NewReadError("path-unavailable", "Document is unavailable")
	}
	target, e := store.Safe(s.Root, basedir+string(filepath.Separator)+relative)
	if e != nil {
		return View{}, NewReadError("permission", "Document link leaves the allowed Store")
	}
	rel, e := filepath.Rel(filepath.Join(s.Root, "documents"), target)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return View{}, NewReadError("permission", "Document link leaves the documents tree")
	}
	data := Object{"resource": filepath.ToSlash(rel), "exists": false, "content": "", "hash": jsonvalue.HashBytes(nil), "title": filepath.Base(target)}
	f, e := os.Open(target)
	if os.IsNotExist(e) {
		view := NewView(pid, state["revision"], "document", data)
		data["read_at"] = view.ReadAt
		return view, nil
	}
	if e != nil {
		return View{}, NewReadError("path-unavailable", "Linked document cannot be read")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return View{}, NewReadError("path-unavailable", "Linked document is not a regular file")
	}
	raw, e := io.ReadAll(io.LimitReader(f, MaxDocumentBytes+1))
	if e != nil {
		return View{}, NewReadError("path-unavailable", "Linked document cannot be read")
	}
	if len(raw) > MaxDocumentBytes {
		return View{}, NewReadError("too-large", "Document exceeds the 2 MiB preview limit")
	}
	if !utf8.Valid(raw) {
		return View{}, NewReadError("invalid", "Document is not UTF-8 text")
	}
	data["exists"], data["content"], data["hash"] = true, string(raw), jsonvalue.HashBytes(raw)
	view := NewView(pid, state["revision"], "document", data)
	data["read_at"] = view.ReadAt
	return view, nil
}
