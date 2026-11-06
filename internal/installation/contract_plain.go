//go:build !contracttest

package installation

import "context"

func testEffect(_ context.Context, _ string, _ []any, original func() error) error { return original() }
