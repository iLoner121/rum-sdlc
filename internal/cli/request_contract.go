//go:build contracttest

package cli

import (
	"context"
	"rumsdlc/internal/app"
	"rumsdlc/internal/service"
)

type RequestHook func(string, string, app.Object) (any, error)
type requestKey struct{}

func WithRequestHook(ctx context.Context, hook RequestHook) context.Context {
	return context.WithValue(ctx, requestKey{}, hook)
}
func request(ctx context.Context, home, method string, params app.Object) (any, error) {
	if hook, ok := ctx.Value(requestKey{}).(RequestHook); ok {
		return hook(home, method, params)
	}
	return service.Request(ctx, home, method, params)
}
