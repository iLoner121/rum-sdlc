//go:build !contracttest

package cli

import (
	"context"
	"rumsdlc/internal/app"
	"rumsdlc/internal/service"
)

func request(ctx context.Context, home, method string, params app.Object) (any, error) {
	return service.Request(ctx, home, method, params)
}
