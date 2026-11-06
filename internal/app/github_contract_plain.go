//go:build !contracttest

package app

import "net/http"

func contractHTTP(client *http.Client) *http.Client { return client }
