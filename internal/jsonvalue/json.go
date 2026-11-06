// Package jsonvalue implements the JSON contract shared with existing RumSDLC
// stores. Numbers retain their original precision; duplicate keys are rejected.
package jsonvalue

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

type Object = map[string]any

func Decode(data []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	v, err := read(d)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("trailing JSON data")
		}
		return nil, err
	}
	return v, nil
}
func read(d *json.Decoder) (any, error) {
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		result := Object{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			s, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("invalid object key")
			}
			if _, ok := result[s]; ok {
				return nil, fmt.Errorf("Duplicate JSON key: %s", s)
			}
			value, err := read(d)
			if err != nil {
				return nil, err
			}
			result[s] = value
		}
		_, err = d.Token()
		return result, err
	case json.Delim('['):
		result := []any{}
		for d.More() {
			value, err := read(d)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		_, err = d.Token()
		return result, err
	default:
		return token, nil
	}
}

// Canonical matches the sorted, compact, UTF-8 JSON used by the Python Store
// preview hash. LegacyJSON is the separate ASCII/spaced encoding used for IDs.
func Canonical(value any) ([]byte, error)  { return encode(value, false, false) }
func LegacyJSON(value any) ([]byte, error) { return encode(value, true, true) }

// DisplayJSON uses readable Unicode and spaces, with deterministic object ordering.
func DisplayJSON(value any) ([]byte, error) { return encode(value, false, true) }
func encode(value any, ascii, spaced bool) ([]byte, error) {
	var out bytes.Buffer
	var write func(any) error
	quote := func(s string) string {
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range s {
			switch r {
			case '"', '\\':
				b.WriteByte('\\')
				b.WriteRune(r)
			case '\b':
				b.WriteString(`\b`)
			case '\f':
				b.WriteString(`\f`)
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			default:
				if r < 32 || ascii && r > 127 {
					if r > 0xffff {
						a, z := utf16.EncodeRune(r)
						fmt.Fprintf(&b, `\u%04x\u%04x`, a, z)
					} else {
						fmt.Fprintf(&b, `\u%04x`, r)
					}
				} else {
					b.WriteRune(r)
				}
			}
		}
		b.WriteByte('"')
		return b.String()
	}
	sep, colon := ",", ":"
	if spaced {
		sep, colon = ", ", ": "
	}
	write = func(v any) error {
		switch x := v.(type) {
		case nil:
			out.WriteString("null")
		case bool:
			out.WriteString(strconv.FormatBool(x))
		case string:
			out.WriteString(quote(x))
		case json.Number:
			if !json.Valid([]byte(x)) {
				return fmt.Errorf("invalid JSON number")
			}
			number, err := pythonNumber(x)
			if err != nil {
				return err
			}
			out.WriteString(number)
		case int:
			out.WriteString(strconv.Itoa(x))
		case int64:
			out.WriteString(strconv.FormatInt(x, 10))
		case []any:
			if x == nil {
				out.WriteString("null")
				break
			}
			out.WriteByte('[')
			for i, e := range x {
				if i > 0 {
					out.WriteString(sep)
				}
				if err := write(e); err != nil {
					return err
				}
			}
			out.WriteByte(']')
		case Object:
			if x == nil {
				out.WriteString("null")
				break
			}
			out.WriteByte('{')
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for i, k := range keys {
				if i > 0 {
					out.WriteString(sep)
				}
				out.WriteString(quote(k))
				out.WriteString(colon)
				if err := write(x[k]); err != nil {
					return err
				}
			}
			out.WriteByte('}')
		default:
			raw, err := json.Marshal(v)
			if err != nil {
				return err
			}
			normalized, err := Decode(raw)
			if err != nil {
				return err
			}
			return write(normalized)
		}
		return nil
	}
	err := write(value)
	return out.Bytes(), err
}
func Hash(value any) (string, error) {
	raw, err := Canonical(value)
	if err != nil {
		return "", err
	}
	return HashBytes(raw), nil
}
func HashBytes(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func Clone(v any) any {
	switch x := v.(type) {
	case Object:
		r := Object{}
		for k, v := range x {
			r[k] = Clone(v)
		}
		return r
	case []any:
		r := make([]any, len(x))
		for i, v := range x {
			r[i] = Clone(v)
		}
		return r
	default:
		return v
	}
}

// Equal follows Python JSON-value equality for no-op detection. In particular,
// 1, 1.0 and true compare equal inside opaque extension payloads, while their
// canonical encodings remain distinct for request-token calculation.
func Equal(a, b any) bool {
	if x, ok := numeric(a); ok {
		if y, ok := numeric(b); ok {
			return x.Cmp(y) == 0
		}
		return false
	}
	switch x := a.(type) {
	case nil:
		if b == nil {
			return true
		}
		if y, ok := b.(Object); ok {
			return y == nil
		}
		if y, ok := b.([]any); ok {
			return y == nil
		}
		return false
	case Object:
		if x == nil {
			return Equal(nil, b)
		}
		y, ok := b.(Object)
		if !ok || y == nil || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			other, exists := y[k]
			if !exists || !Equal(v, other) {
				return false
			}
		}
		return true
	case []any:
		if x == nil {
			return Equal(nil, b)
		}
		y, ok := b.([]any)
		if !ok || y == nil || len(x) != len(y) {
			return false
		}
		for i, v := range x {
			if !Equal(v, y[i]) {
				return false
			}
		}
		return true
	case string:
		y, ok := b.(string)
		return ok && x == y
	}
	x, e := Canonical(a)
	if e != nil {
		return false
	}
	y, e := Canonical(b)
	return e == nil && bytes.Equal(x, y)
}
func numeric(v any) (*big.Rat, bool) {
	switch x := v.(type) {
	case bool:
		if x {
			return big.NewRat(1, 1), true
		}
		return big.NewRat(0, 1), true
	case int:
		return new(big.Rat).SetInt64(int64(x)), true
	case int64:
		return new(big.Rat).SetInt64(x), true
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, false
		}
		return new(big.Rat).SetFloat64(x), true
	case json.Number:
		if strings.ContainsAny(string(x), ".eE") {
			f, e := strconv.ParseFloat(string(x), 64)
			if e != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				return nil, false
			}
			return new(big.Rat).SetFloat64(f), true
		}
		n, ok := new(big.Int).SetString(string(x), 10)
		if !ok {
			return nil, false
		}
		return new(big.Rat).SetInt(n), true
	}
	return nil, false
}

// Python hashes decoded JSON numbers, not their original spelling. Preserve
// arbitrary precision integers and match its finite float representation.
func pythonNumber(n json.Number) (string, error) {
	raw := string(n)
	if !strings.ContainsAny(raw, ".eE") {
		integer, ok := new(big.Int).SetString(raw, 10)
		if !ok {
			return "", fmt.Errorf("invalid integer")
		}
		return integer.String(), nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return "", fmt.Errorf("non-finite JSON number")
	}
	if value == 0 {
		if math.Signbit(value) {
			return "-0.0", nil
		}
		return "0.0", nil
	}
	exponential := strconv.FormatFloat(value, 'e', -1, 64)
	at := strings.IndexByte(exponential, 'e')
	exponent, _ := strconv.Atoi(exponential[at+1:])
	if exponent >= -4 && exponent < 16 {
		fixed := strconv.FormatFloat(value, 'f', -1, 64)
		if !strings.Contains(fixed, ".") {
			fixed += ".0"
		}
		return fixed, nil
	}
	return exponential, nil
}
