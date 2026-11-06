//go:build contracttest

package app

import (
	"io"
	"net/http"
	"os"
	"strings"
)

type contractRoundTrip func(*http.Request) (*http.Response, error)

func (f contractRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Mirrors the original installed-CLI Fake gh fixture. It is absent from releases.
func contractHTTP(client *http.Client) *http.Client {
	if os.Getenv("RUM_TEST_OFFLINE_HUB") != "1" {
		return client
	}
	client.Transport = contractRoundTrip(func(r *http.Request) (*http.Response, error) {
		status, body := 200, `[]`
		if r.URL.Path == "/user" {
			body = `{"id":7,"login":"alice","name":"Alice"}`
		} else if r.URL.Path != "/user/repos" {
			status = 404
			body = `{"error":"unexpected fixture endpoint"}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	return client
}
