//go:build !contracttest

package main

import (
	"context"
	"rumsdlc/internal/app"
)

func fixtureCommand(_ []string) (bool, error) { return false, nil }

func configureHooks(ctx context.Context, _ *app.Device, _ *callbackHub, _ map[string]any) context.Context {
	return ctx
}

func configureInstallHooks(ctx context.Context, _ *callbackHub, _ map[string]any) context.Context {
	return ctx
}
