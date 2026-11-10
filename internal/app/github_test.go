package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A real child executable, so credential isolation is tested at the process
// boundary on Windows as well as Unix. This helper exists only in test binaries.
func TestMain(m *testing.M) {
	if os.Getenv("RUM_TEST_CREDENTIAL_HELPER") == "1" {
		if strings.Join(os.Args[1:], " ") != "auth token --hostname github.com --user alice" || os.Getenv("GH_TOKEN") != "" || os.Getenv("GITHUB_TOKEN") != "" || os.Getenv("GH_DEBUG") != "" {
			os.Exit(3)
		}
		fmt.Println("fixture-secret")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
func reply(request *http.Request, value any) *http.Response {
	raw, _ := json.Marshal(value)
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw))), Request: request}
}
func TestSelectedCredentialIsScopedToChildAndRequest(t *testing.T) {
	temp := t.TempDir()
	name := "gh"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(exe)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(temp, name), raw, 0755); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", temp)
	t.Setenv("RUM_TEST_CREDENTIAL_HELPER", "1")
	t.Setenv("GH_TOKEN", "other-account")
	t.Setenv("GITHUB_TOKEN", "other-account-2")
	t.Setenv("GH_DEBUG", "api")
	g := NewGitHub()
	g.Account = "alice"
	g.Client.Transport = testTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Error("selected credential not used")
		}
		return reply(r, Object{"id": 7, "login": "alice"}), nil
	})
	user, e := g.User(context.Background())
	if e != nil || user["login"] != "alice" {
		t.Fatalf("%v: %v", user, e)
	}
	if os.Getenv("GH_TOKEN") != "other-account" || os.Getenv("GH_DEBUG") != "api" {
		t.Fatal("parent environment was mutated")
	}
}
func TestGitHubPaginationCreationManifestAndErrors(t *testing.T) {
	t.Setenv("GH_TOKEN", "fixture-secret")
	g := NewGitHub()
	pages := 0
	created := false
	g.Client.Transport = testTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer fixture-secret" || r.URL.Host != "api.github.com" {
			t.Fatal("invalid credential scope")
		}
		switch {
		case r.URL.Path == "/user/repos" && r.Method == "GET":
			pages++
			count := 100
			if pages == 2 {
				count = 1
			}
			if r.URL.Query().Get("page") != fmt.Sprint(pages) {
				t.Error("invalid page")
			}
			batch := []any{}
			for i := 0; i < count; i++ {
				batch = append(batch, Object{"id": i, "name": "repo"})
			}
			return reply(r, batch), nil
		case r.URL.Path == "/user/repos" && r.Method == "POST":
			var body Object
			if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
				t.Fatal(e)
			}
			if body["private"] != true || body["auto_init"] != false || body["name"] != "RumSDLC_P-test" {
				t.Fatalf("unsafe create: %v", body)
			}
			created = true
			return reply(r, Object{"name": "RumSDLC_P-test"}), nil
		case strings.Contains(r.URL.Path, "/contents/"):
			if r.URL.Query().Get("ref") != "branch/中文" {
				t.Error("ref not encoded")
			}
			return reply(r, Object{"type": "file", "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(`{"project_id":"P-test"}`))}), nil
		default:
			response := reply(r, Object{"error": "fixture-secret must never escape"})
			response.StatusCode = 403
			return response, nil
		}
	})
	ctx := context.Background()
	repos, e := g.Repositories(ctx)
	if e != nil || len(repos) != 101 || pages != 2 {
		t.Fatalf("pagination: %d %v", len(repos), e)
	}
	if _, e = g.Create(ctx, "RumSDLC_P-test"); e != nil || !created {
		t.Fatal(e)
	}
	manifest, e := g.Manifest(ctx, "alice/RumSDLC_P-test", "branch/中文")
	if e != nil || manifest["project_id"] != "P-test" {
		t.Fatalf("manifest: %v %v", manifest, e)
	}
	if _, e = g.Repository(ctx, "alice/denied"); e == nil || strings.Contains(e.Error(), "fixture-secret") {
		t.Fatalf("error redaction: %v", e)
	}
	if _, e = g.URL(Object{"full_name": "https://evil.test/repo"}); e == nil {
		t.Fatal("foreign credential host accepted")
	}
}
func TestReviewIdentityAndMergeEvidence(t *testing.T) {
	address := "https://github.com/example/code/pull/7"
	response := Object{"number": 7, "html_url": address, "state": "closed", "merged": true, "merged_at": "2026-11-05T00:00:00Z", "merge_commit_sha": strings.Repeat("a", 40), "base": Object{"ref": "main", "sha": strings.Repeat("b", 40), "repo": Object{"full_name": "example/code"}}, "head": Object{"ref": "feature", "sha": strings.Repeat("c", 40)}}
	for _, change := range []Object{{}, {"merge_commit_sha": nil}, {"number": 8}, {"merged": "true"}, {"head": nil}} {
		value := clone(response).(map[string]any)
		for key, v := range change {
			value[key] = v
		}
		e := attempt(func() {
			facts := validateReview(value, address, "example/code", "7")
			if facts["merged"] != true {
				t.Error("merge fact lost")
			}
		})
		if (len(change) == 0) != (e == nil) {
			t.Fatalf("change %v: %v", change, e)
		}
	}
}

func TestGitHubManifestErrorClassification(t *testing.T) {
	t.Setenv("GH_TOKEN", "fixture-secret")
	for _, code := range []int{404, 403, 429, 500} {
		g := NewGitHub()
		g.Client.Transport = testTransport(func(r *http.Request) (*http.Response, error) {
			response := reply(r, Object{"message": "fixture-secret must not escape"})
			response.StatusCode = code
			return response, nil
		})
		_, err := g.Manifest(context.Background(), "alice/RumSDLC_test", "main")
		want := "unavailable"
		if code == 404 {
			want = "missing"
		}
		if err == nil || manifestReadKind(err) != want || strings.Contains(err.Error(), "fixture-secret") {
			t.Fatal(code, err)
		}
	}
	g := NewGitHub()
	g.Client.Transport = testTransport(func(r *http.Request) (*http.Response, error) {
		return reply(r, Object{"type": "dir", "encoding": "base64"}), nil
	})
	if _, err := g.Manifest(context.Background(), "alice/RumSDLC_test", "main"); err == nil || manifestReadKind(err) != "invalid" {
		t.Fatal(err)
	}
}

func TestGitHubLegacyProjectDisplayName(t *testing.T) {
	t.Setenv("GH_TOKEN", "fixture-secret")
	g := NewGitHub()
	g.Client.Transport = testTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/repos/alice/RumSDLC_old/contents/project-config.json" || r.URL.Query().Get("ref") != "branch/name" {
			t.Fatal(r.URL)
		}
		return reply(r, Object{"type": "file", "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(`{"name":"Old display"}`))}), nil
	})
	name, err := g.ProjectDisplayName(context.Background(), "alice/RumSDLC_old", "branch/name")
	if err != nil || name != "Old display" {
		t.Fatal(name, err)
	}
}
