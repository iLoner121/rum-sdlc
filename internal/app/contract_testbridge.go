//go:build contracttest

package app

import (
	"context"
	"encoding/json"
	"rumsdlc/internal/jsonvalue"
)

// ContractHook exists only in the original-test transport build. A Python test
// may inject I/O failures around a native continuation; business logic never
// falls back to Python. Release builds do not contain this hook.
type ContractHook func(context.Context, string, []any, func() (any, error)) (any, error)
type contractKey struct{}

func WithContractHook(ctx context.Context, hook ContractHook) context.Context {
	return context.WithValue(ctx, contractKey{}, hook)
}
func contract[T any](ctx context.Context, name string, args []any, original func() T) T {
	hook, _ := ctx.Value(contractKey{}).(ContractHook)
	if hook == nil {
		return original()
	}
	proceed := func() (result any, err error) { defer guard(&err); return original(), nil }
	value, e := hook(ctx, name, args, proceed)
	must(e)
	if typed, ok := value.(T); ok {
		return typed
	}
	raw, e := json.Marshal(value)
	must(e)
	var output T
	// Decode flexible JSON values without converting integral tokens to float64.
	if _, ok := any(output).(Object); ok {
		v, e := jsonvalue.Decode(raw)
		must(e)
		return v.(T)
	}
	must(json.Unmarshal(raw, &output))
	return output
}
