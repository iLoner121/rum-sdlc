//go:build contracttest

package installation

import "context"

// ContractHook is compiled only into the original-test transport binary.
type ContractHook func(string, []any, func() error) error
type contractKey struct{}

func WithContractHook(ctx context.Context, hook ContractHook) context.Context {
	return context.WithValue(ctx, contractKey{}, hook)
}
func testEffect(ctx context.Context, name string, args []any, original func() error) error {
	hook, _ := ctx.Value(contractKey{}).(ContractHook)
	if hook == nil {
		return original()
	}
	return hook(name, args, original)
}
