//go:build contracttest

package main

import (
	"context"
	"fmt"
	"rumsdlc/internal/app"
	"rumsdlc/internal/installation"
	"rumsdlc/internal/jsonvalue"
)

func (h *callbackHub) effect(name string, args []any, original func() (any, error)) (any, error) {
	if e := h.output.Encode(map[string]any{"effect": name, "args": args}); e != nil {
		return nil, e
	}
	for {
		raw, e := h.input.ReadBytes('\n')
		if e != nil {
			return nil, e
		}
		value, e := jsonvalue.Decode(raw)
		if e != nil {
			return nil, e
		}
		response, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Invalid effect response")
		}
		if response["continue"] == true {
			result, e := original()
			message := map[string]any{"continuation_result": result}
			if e != nil {
				message = map[string]any{"continuation_error": e.Error()}
			}
			if e = h.output.Encode(message); e != nil {
				return nil, e
			}
			continue
		}
		if response["error"] != nil {
			return nil, fmt.Errorf("%v", response["error"])
		}
		return response["result"], nil
	}
}
func configureHooks(ctx context.Context, device *app.Device, hub *callbackHub, request map[string]any) context.Context {
	enabled := map[string]bool{}
	names, _ := request["hooks"].([]any)
	for _, name := range names {
		if s, ok := name.(string); ok {
			enabled[s] = true
		}
	}
	if enabled["observation.read_review"] {
		device.Review = func(_ context.Context, url string) (app.Object, error) {
			v, e := hub.effect("observation.read_review", []any{url}, func() (any, error) { return nil, fmt.Errorf("Test review reader needs an explicit fixture") })
			if e != nil {
				return nil, e
			}
			result, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("Invalid review fixture")
			}
			return result, nil
		}
	}
	return app.WithContractHook(ctx, func(_ context.Context, name string, args []any, original func() (any, error)) (any, error) {
		if !enabled[name] {
			return original()
		}
		return hub.effect(name, args, original)
	})
}

func configureInstallHooks(ctx context.Context, hub *callbackHub, request map[string]any) context.Context {
	enabled := map[string]bool{}
	names, _ := request["hooks"].([]any)
	for _, name := range names {
		if s, ok := name.(string); ok {
			enabled[s] = true
		}
	}
	return installation.WithContractHook(ctx, func(name string, args []any, original func() error) error {
		if !enabled[name] {
			return original()
		}
		_, e := hub.effect(name, args, func() (any, error) { return nil, original() })
		return e
	})
}
