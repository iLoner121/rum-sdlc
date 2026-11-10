// Package update owns device-local update information, never project state.
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"rumsdlc/internal/release"
	"strings"
	"time"
)

const Repository = "iLoner121/rum-sdlc"
const MaxDownload int64 = 512 * 1024 * 1024

type Failure struct {
	Code    string    `json:"code"`
	Message string    `json:"message"`
	RetryAt time.Time `json:"retry_at,omitempty"`
}

func (e *Failure) Error() string  { return e.Code + ": " + e.Message }
func fail(code, msg string) error { return &Failure{Code: code, Message: msg} }

type Asset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}
type Target struct {
	ID         int64   `json:"id"`
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	URL        string  `json:"html_url"`
	Assets     []Asset `json:"assets"`
}

func (t Target) Version() string { return strings.TrimPrefix(t.Tag, "v") }
func (t Target) Asset(name string) (Asset, error) {
	var found Asset
	for _, a := range t.Assets {
		if a.Name == name {
			if found.Name != "" {
				return Asset{}, fail("invalid-release", "Duplicate asset")
			}
			found = a
		}
	}
	if found.Name == "" {
		return found, fail("missing-asset", name)
	}
	return found, nil
}
func PackageName(version, goos, arch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return "rumsdlc-" + version + "-" + goos + "-" + arch + ext
}
func Supported(goos, arch string) bool {
	return (goos == "darwin" || goos == "linux" || goos == "windows") && (arch == "amd64" || arch == "arm64")
}

type Client struct {
	HTTP  *http.Client
	Base  string
	Token string
}

func NewClient() *Client {
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	return &Client{Token: token, Base: "https://api.github.com", HTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: redirect}}
}
func allowed(host string) bool {
	return host == "api.github.com" || host == "github.com" || host == "release-assets.githubusercontent.com" || host == "objects.githubusercontent.com"
}
func redirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fail("redirect", "Too many redirects")
	}
	req.Header.Del("Authorization")
	if req.URL.Scheme != "https" || !allowed(req.URL.Hostname()) {
		return fail("redirect", "Unexpected release download host")
	}
	return nil
}
func (c *Client) request(ctx context.Context, raw, etag string, limit int64, download bool) ([]byte, string, error) {
	u, e := url.Parse(raw)
	if e != nil {
		return nil, "", e
	}
	base, _ := url.Parse(c.Base)
	// A custom HTTP base is a test injection only; production has no URL override.
	if u.Scheme != "https" || !allowed(u.Hostname()) {
		if base == nil || base.Host != u.Host || base.Scheme != "http" {
			return nil, "", fail("source", "Unexpected release URL")
		}
	}
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return nil, "", e
	}
	req.Header.Set("User-Agent", "RumSDLC/"+release.Version)
	req.Header.Set("Accept", "application/vnd.github+json")
	if u.Hostname() == "api.github.com" && c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	client := c.HTTP
	if client == nil {
		client = NewClient().HTTP
	}
	if download {
		copy := *client
		copy.Timeout = 10 * time.Minute
		client = &copy
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, "", fail("network", "Release request failed; retry when connected")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 304 {
		return nil, etag, nil
	}
	if resp.StatusCode == 404 {
		return nil, "", fail("no-release", "No published release or asset is available")
	}
	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		retry := time.Now().Add(time.Hour)
		if v := resp.Header.Get("Retry-After"); v != "" {
			if d, e := time.ParseDuration(v + "s"); e == nil && d > 0 {
				retry = time.Now().Add(d)
			}
		}
		return nil, "", &Failure{Code: "rate-limited", Message: "GitHub refused this request; retry later or use an existing GitHub token", RetryAt: retry}
	}
	if resp.StatusCode != 200 {
		return nil, "", fail("http", "Release request returned HTTP "+fmt.Sprint(resp.StatusCode))
	}
	if resp.ContentLength > limit {
		return nil, "", fail("size-limit", "Release response exceeds size limit")
	}
	rawBytes, e := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if e != nil {
		return nil, "", fail("download", "Incomplete response")
	}
	if int64(len(rawBytes)) > limit {
		return nil, "", fail("size-limit", "Release response exceeds size limit")
	}
	return rawBytes, resp.Header.Get("ETag"), nil
}
func (c *Client) Latest(ctx context.Context, etag string) (*Target, string, error) {
	raw, tag, e := c.request(ctx, c.Base+"/repos/"+Repository+"/releases/latest", etag, 2*1024*1024, false)
	if e != nil {
		return nil, "", e
	}
	if raw == nil {
		return nil, tag, nil
	}
	var t Target
	if json.Unmarshal(raw, &t) != nil {
		return nil, "", fail("invalid-release", "Invalid release response")
	}
	v, e := release.Parse(t.Version())
	if e != nil || len(v.Pre) > 0 || t.Draft || t.Prerelease || t.ID <= 0 {
		return nil, "", fail("invalid-release", "Expected a published stable semantic version")
	}
	if _, e = t.Asset("checksums.json"); e != nil {
		return nil, "", e
	}
	return &t, tag, nil
}
func Classify(current release.Identity, t Target) (string, error) {
	if current.Channel != "stable" {
		return "channel-switch-required", nil
	}
	n, e := release.Compare(t.Version(), current.Version)
	if e != nil {
		return "unknown-current", e
	}
	if n > 0 {
		return "update-available", nil
	}
	if n < 0 {
		return "local-newer", nil
	}
	return "up-to-date", nil
}

type Checksums struct {
	Version string            `json:"version"`
	Commit  string            `json:"source_commit"`
	Channel string            `json:"channel"`
	Files   map[string]string `json:"files"`
}

// Download fixes every asset to one release response, verifies its archive, and
// extracts only the expected executable. No archive paths are written to disk.
func (c *Client) Download(ctx context.Context, t Target, goos, arch, dir string) (string, Checksums, error) {
	var sums Checksums
	if !Supported(goos, arch) {
		return "", sums, fail("unsupported", "Unsupported system or architecture")
	}
	a, e := t.Asset("checksums.json")
	if e != nil {
		return "", sums, e
	}
	raw, _, e := c.request(ctx, a.URL, "", 2*1024*1024, true)
	if e != nil {
		return "", sums, e
	}
	if json.Unmarshal(raw, &sums) != nil || sums.Version != t.Version() || (sums.Channel != "" && sums.Channel != "stable") {
		return "", sums, fail("invalid-checksums", "Checksum version does not match selected release")
	}
	name := PackageName(t.Version(), goos, arch)
	a, e = t.Asset(name)
	if e != nil {
		return "", sums, e
	}
	expected := sums.Files[name]
	if b, e := hex.DecodeString(expected); e != nil || len(b) != 32 {
		return "", sums, fail("invalid-checksums", "Missing SHA-256 for selected package")
	}
	if a.Size > MaxDownload || a.Size <= 0 {
		return "", sums, fail("size-limit", "Invalid package size")
	}
	raw, _, e = c.request(ctx, a.URL, "", MaxDownload, true)
	if e != nil {
		return "", sums, e
	}
	if int64(len(raw)) != a.Size {
		return "", sums, fail("download", "Asset size changed or download incomplete")
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != expected {
		return "", sums, fail("checksum-mismatch", "Package SHA-256 does not match release")
	}
	binary := "rum"
	if goos == "windows" {
		binary += ".exe"
	}
	payload, e := extract(raw, binary, goos == "windows")
	if e != nil {
		return "", sums, e
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", sums, e
	}
	path := filepath.Join(dir, binary)
	if e = os.WriteFile(path, payload, 0700); e != nil {
		return "", sums, e
	}
	return path, sums, nil
}
func extract(raw []byte, name string, windows bool) ([]byte, error) {
	var content []byte
	count := 0
	accept := func(n string, regular bool, size int64, r io.Reader) error {
		if n != name || !regular || size <= 0 || size > MaxDownload {
			return fail("invalid-archive", "Archive must contain only the expected executable")
		}
		count++
		if count > 1 {
			return fail("invalid-archive", "Duplicate executable")
		}
		b, e := io.ReadAll(io.LimitReader(r, MaxDownload+1))
		if e != nil {
			return e
		}
		if int64(len(b)) != size {
			return fail("invalid-archive", "Invalid expanded size")
		}
		content = b
		return nil
	}
	if windows {
		z, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if e != nil {
			return nil, e
		}
		for _, f := range z.File {
			r, e := f.Open()
			if e != nil {
				return nil, e
			}
			e = accept(f.Name, f.Mode().IsRegular(), int64(f.UncompressedSize64), r)
			r.Close()
			if e != nil {
				return nil, e
			}
		}
	} else {
		g, e := gzip.NewReader(bytes.NewReader(raw))
		if e != nil {
			return nil, e
		}
		defer g.Close()
		t := tar.NewReader(g)
		for {
			h, e := t.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, e
			}
			if e = accept(h.Name, h.Typeflag == tar.TypeReg, h.Size, t); e != nil {
				return nil, e
			}
		}
	}
	if count != 1 {
		return nil, fail("invalid-archive", "Executable is missing")
	}
	return content, nil
}

// Confirm rejects a withdrawn release or changed asset identities between
// selection and execution. It never silently substitutes a newer release.
func (c *Client) Confirm(ctx context.Context, selected Target) error {
	raw, _, e := c.request(ctx, c.Base+"/repos/"+Repository+"/releases/"+fmt.Sprint(selected.ID), "", 2*1024*1024, false)
	if e != nil {
		return e
	}
	var current Target
	if json.Unmarshal(raw, &current) != nil || current.ID != selected.ID || current.Tag != selected.Tag || current.Draft || current.Prerelease {
		return fail("release-changed", "Selected release was withdrawn or changed")
	}
	for _, asset := range selected.Assets {
		a, e := current.Asset(asset.Name)
		if e != nil || a.ID != asset.ID || a.Size != asset.Size || a.URL != asset.URL {
			return fail("release-changed", "Selected release assets changed")
		}
	}
	return nil
}
