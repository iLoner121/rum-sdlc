// Package webui serves embedded browsing resources and a restricted read API.
package webui

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"rumsdlc/internal/app"
	"rumsdlc/internal/fsutil"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed assets
var assets embed.FS

type RemoteReader interface {
	ReadRemoteProjects(context.Context) (app.View, error)
}
type Options struct {
	Port  int
	Ready func(string)
}

func Serve(ctx context.Context, reader *app.Reader, o Options) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if o.Port < 0 || o.Port > 65535 {
		return fmt.Errorf("Invalid UI port")
	}
	lease, e := fsutil.Lease(reader.Home, false)
	if e != nil {
		return e
	}
	defer lease.Close()
	listener, e := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", o.Port))
	if e != nil {
		return e
	}
	defer listener.Close()
	host := listener.Addr().String()
	var requests sync.WaitGroup
	var requestMu sync.Mutex
	stopping := false
	handler := Handler(reader, host)
	tracked := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMu.Lock()
		if stopping {
			requestMu.Unlock()
			http.Error(w, "UI is stopping", http.StatusServiceUnavailable)
			return
		}
		requests.Add(1)
		requestMu.Unlock()
		defer requests.Done()
		handler.ServeHTTP(w, r)
	})
	server := &http.Server{Handler: tracked, BaseContext: func(net.Listener) context.Context { return ctx }, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 75 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		requestMu.Lock()
		stopping = true
		requestMu.Unlock()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if server.Shutdown(shutdown) != nil {
			server.Close()
		}
		requests.Wait()
	}()
	if o.Ready != nil {
		o.Ready("http://" + host + "/")
	}
	err := server.Serve(listener)
	if !errors.Is(err, http.ErrServerClosed) {
		cancel()
		<-stopped
		return err
	}
	<-stopped
	return nil
}
func writeError(w http.ResponseWriter, e error) {
	code, message, status := "unknown", "Unable to read this resource", http.StatusInternalServerError
	var re *app.ReadError
	if errors.As(e, &re) {
		code, message = re.Code, re.Message
		switch code {
		case "missing":
			status = 404
		case "invalid":
			status = 400
		case "stale", "busy":
			status = 409
		case "authentication", "permission", "account-changed":
			status = 403
		case "too-large":
			status = 413
		case "path-unavailable", "identity-invalid", "unsupported":
			status = 422
		case "network":
			status = 502
		case "rate-limited":
			status = 429
		case "cancelled":
			status = 408
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(app.Object{"error": app.Object{"code": code, "message": message}})
}
func Handler(reader *app.Reader, host string) http.Handler {
	files, _ := fs.Sub(assets, "assets")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w = &headWriter{ResponseWriter: w}
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		if r.Host != host {
			writeError(w, app.NewReadError("permission", "Invalid local Host"))
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+host {
			writeError(w, app.NewReadError("permission", "Cross-origin requests are not permitted"))
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			writeError(w, app.NewReadError("permission", "Cross-site requests are not permitted"))
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			v, e := route(r, reader)
			if e != nil {
				writeError(w, e)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			json.NewEncoder(w).Encode(v)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		name = strings.TrimPrefix(name, "assets/")
		if name == "" {
			name = "index.html"
		}
		if !fs.ValidPath(name) || strings.Contains(name, "\\") {
			http.NotFound(w, r)
			return
		}
		info, e := fs.Stat(files, name)
		if e != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		raw, e := fs.ReadFile(files, name)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(name, ".js") {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		}
		http.ServeContent(w, r, path.Base(name), time.Time{}, strings.NewReader(string(raw)))
	})
}

type headWriter struct{ http.ResponseWriter }

func (w *headWriter) Write(p []byte) (int, error) { return len(p), nil }
func route(r *http.Request, reader *app.Reader) (app.View, error) {
	raw := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	if raw == r.URL.Path {
		return app.View{}, app.NewReadError("missing", "API route does not exist")
	}
	parts := strings.Split(raw, "/")
	q := r.URL.Query()
	revision := q.Get("revision")
	f := app.TopicFilter{Scope: q.Get("scope"), WorksetID: q.Get("workset_id"), MilestoneID: q.Get("milestone_id"), LineID: q.Get("line_id"), Status: q.Get("status"), Query: q.Get("q"), Basis: q.Get("basis"), Revision: revision}
	for key, target := range map[string]*int{"offset": &f.Offset, "limit": &f.Limit} {
		if q.Has(key) {
			n, e := strconv.Atoi(q.Get(key))
			if e != nil || n < 0 || (key == "limit" && (n < 1 || n > 200)) {
				return app.View{}, app.NewReadError("invalid", "Invalid page range")
			}
			*target = n
		}
	}
	if len(parts) == 1 {
		switch parts[0] {
		case "device":
			return reader.ReadDevice()
		case "projects":
			return reader.ReadProjects()
		case "remote-projects":
			if remote, ok := any(reader).(RemoteReader); ok {
				ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
				defer cancel()
				return remote.ReadRemoteProjects(ctx)
			}
			return app.View{}, app.NewReadError("authentication", "Remote browsing is unavailable")
		}
	}
	if len(parts) < 3 || parts[0] != "projects" || parts[1] == "" {
		return app.View{}, app.NewReadError("missing", "API route does not exist")
	}
	pid, e := url.PathUnescape(parts[1])
	if e != nil {
		return app.View{}, app.NewReadError("invalid", "Invalid Project ID")
	}
	if len(parts) == 4 && parts[2] == "topics" {
		return reader.ReadTopic(pid, parts[3], revision)
	}
	if len(parts) != 3 {
		return app.View{}, app.NewReadError("missing", "API route does not exist")
	}
	switch parts[2] {
	case "overview":
		return reader.ReadOverview(pid, revision)
	case "milestones":
		return reader.ReadPlanning(pid, "milestones", revision, f)
	case "iterations":
		return reader.ReadPlanning(pid, "worksets", revision, f)
	case "topics":
		return reader.ReadTopics(pid, f)
	case "pool":
		f.Scope = "pool"
		return reader.ReadTopics(pid, f)
	case "documents":
		return reader.ReadDocument(pid, q.Get("table"), q.Get("id"), q.Get("field"), revision)
	case "document-link":
		if q.Get("resource") != "" {
			return reader.ReadDocumentResource(pid, q.Get("resource"), q.Get("href"), revision)
		}
		return reader.ReadDocumentLink(pid, q.Get("table"), q.Get("id"), q.Get("field"), q.Get("href"), revision)
	}
	return app.View{}, app.NewReadError("missing", "API route does not exist")
}
