// core-contract is a test transport, not the product CLI. It contains no Python.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"rumsdlc/internal/core"
	"rumsdlc/internal/jsonvalue"
)

func call(raw []byte) (response map[string]any) {
	// A malformed external value must never terminate the contract test stream.
	defer func() {
		if p := recover(); p != nil {
			response = map[string]any{"error": fmt.Sprint(p), "panic": true}
		}
	}()
	value, err := jsonvalue.Decode(raw)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	request, ok := value.(map[string]any)
	if !ok {
		return map[string]any{"error": "Expected object"}
	}
	var state core.Object
	if request["state"] != nil {
		state, ok = request["state"].(map[string]any)
		if !ok {
			return map[string]any{"error": "Expected state object"}
		}
	}
	switch request["operation"] {
	case "validate":
		err = core.Validate(state)
		if err == nil {
			return map[string]any{"ok": true}
		}
	case "query":
		params, ok := request["params"].(map[string]any)
		if !ok {
			return map[string]any{"error": "Expected query parameters"}
		}
		result, e := core.Query(state, params)
		err = e
		if err == nil {
			return map[string]any{"result": result}
		}
	case "dispatch":
		command, ok := request["request"].(map[string]any)
		if !ok {
			return map[string]any{"error": "Expected request object"}
		}
		after, result, e := core.Dispatch(state, command)
		err = e
		if err == nil {
			err = core.Validate(after)
		}
		if err == nil {
			hash, e := jsonvalue.Hash(map[string]any{"before": state, "request": command, "after": after})
			if e != nil {
				err = e
			} else {
				return map[string]any{"state": after, "result": result, "revision": hash}
			}
		}
	default:
		err = fmt.Errorf("Unknown operation")
	}
	return map[string]any{"error": err.Error()}
}
func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 65536), 64*1024*1024)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		if err := encoder.Encode(call(scanner.Bytes())); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
