package jsonvalue

import "testing"

func TestPythonCanonicalNumbers(t *testing.T) {
	for raw, want := range map[string]string{"1e0": "1.0", "1e15": "1000000000000000.0", "1e16": "1e+16", "1e-4": "0.0001", "1e-5": "1e-05", "-0": "0", "-0.0": "-0.0", "900719925474099312345": "900719925474099312345"} {
		v, e := Decode([]byte(raw))
		if e != nil {
			t.Fatal(e)
		}
		got, e := Canonical(v)
		if e != nil || string(got) != want {
			t.Errorf("%s -> %s, %v; want %s", raw, got, e, want)
		}
	}
}
func TestRejectDuplicateAndTrailingJSON(t *testing.T) {
	for _, raw := range []string{`{"a":1,"a":2}`, `{"a":{"x":1,"x":2}}`, `{} []`, `NaN`, `{"a":Infinity}`} {
		if _, e := Decode([]byte(raw)); e == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
func TestUnicodeHashAndLegacyEncoding(t *testing.T) {
	v := Object{"测试": "😀<>&\n"}
	raw, e := Canonical(v)
	if e != nil || string(raw) != `{"测试":"😀<>&\n"}` {
		t.Fatalf("%s %v", raw, e)
	}
	raw, e = LegacyJSON(v)
	if e != nil || string(raw) != `{"\u6d4b\u8bd5": "\ud83d\ude00<>&\n"}` {
		t.Fatalf("%s %v", raw, e)
	}
}
func TestNilObjectIsJSONNull(t *testing.T) {
	var object Object
	raw, e := Canonical(Object{"before": object})
	if e != nil || string(raw) != `{"before":null}` {
		t.Fatalf("%s %v", raw, e)
	}
}

func TestPythonEqualityDoesNotChangeTokenEncoding(t *testing.T) {
	a, _ := Decode([]byte(`{"value":1}`))
	b, _ := Decode([]byte(`{"value":1.0}`))
	c, _ := Decode([]byte(`{"value":true}`))
	if !Equal(a, b) || !Equal(a, c) {
		t.Fatal("numeric no-op equality differs")
	}
	ha, _ := Hash(a)
	hb, _ := Hash(b)
	if ha == hb {
		t.Fatal("request token lost numeric representation")
	}
}
