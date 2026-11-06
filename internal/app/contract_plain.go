//go:build !contracttest

package app

import "context"

// Production builds contain no test transport or externally configurable hooks.
func contract[T any](_ context.Context, _ string, _ []any, original func() T) T { return original() }
