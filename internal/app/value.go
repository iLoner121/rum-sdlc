// Package app implements device operations independently of the CLI and service.
package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/release"
	"sort"
	"strconv"
	"strings"
)

type Object = jsonvalue.Object

const Version = release.Version
const Manifest = ".rumsdlc-project.json"
const RepositoryPrefix = "RumSDLC_"

type failure struct{ error }

func guard(err *error) {
	if p := recover(); p != nil {
		if f, ok := p.(failure); ok {
			*err = f.error
		} else {
			panic(p)
		}
	}
}
func require(ok bool, message string) {
	if !ok {
		panic(failure{errors.New(message)})
	}
}
func must(err error) {
	if err != nil {
		panic(failure{err})
	}
}
func object(v any, label string) Object {
	x, ok := v.(map[string]any)
	require(ok, label+" must be a JSON object")
	return x
}
func array(v any, label string) []any {
	x, ok := v.([]any)
	require(ok, label+" must be an array")
	return x
}
func str(v any) string { x, _ := v.(string); return x }
func text(v any, label string) string {
	s, ok := v.(string)
	require(ok && strings.TrimSpace(s) != "", label+" must be nonempty text")
	return s
}
func truth(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case Object:
		return len(x) > 0
	case []any:
		return len(x) > 0
	default:
		return !jsonvalue.Equal(v, 0)
	}
}
func integer(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case json.Number:
		n, e := x.Int64()
		must(e)
		return n
	}
	panic(failure{fmt.Errorf("Expected integer")})
}
func idString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return ""
}
func equal(a, b any) bool        { return jsonvalue.Equal(a, b) }
func copyObject(v Object) Object { return jsonvalue.Clone(v).(Object) }
func clone(v any) any            { return jsonvalue.Clone(v) }
func keys(v Object) []string {
	k := make([]string, 0, len(v))
	for key := range v {
		k = append(k, key)
	}
	sort.Strings(k)
	return k
}
func has(v Object, key string) bool { _, ok := v[key]; return ok }
func oneOf(value, choices string) bool {
	for _, s := range strings.Fields(choices) {
		if value == s {
			return true
		}
	}
	return false
}
func exact(v Object, names, message string) {
	fields := strings.Fields(names)
	require(len(fields) == len(v), message)
	for _, key := range fields {
		require(has(v, key), message)
	}
}
func only(v Object, names, message string) {
	for key := range v {
		require(oneOf(key, names), message)
	}
}
func fingerprint(value any) string { h, e := jsonvalue.Hash(value); must(e); return h }
func getObject(v Object, key string) Object {
	if v[key] == nil {
		return Object{}
	}
	return object(v[key], key)
}

func fingerprintBytes(value string) string { return jsonvalue.HashBytes([]byte(value)) }
