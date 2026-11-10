package release

import "testing"

func TestSemanticPrecedence(t *testing.T) {
	ordered := []string{"0.9.0", "0.10.0-alpha", "0.10.0-alpha.1", "0.10.0-alpha.beta", "0.10.0-beta", "0.10.0-beta.2", "0.10.0-beta.11", "0.10.0-rc.1", "0.10.0", "1.0.0", "1000000000000000000000000.0.0"}
	for i := 1; i < len(ordered); i++ {
		n, e := Compare(ordered[i-1], ordered[i])
		if e != nil || n >= 0 {
			t.Fatalf("%s < %s: %d %v", ordered[i-1], ordered[i], n, e)
		}
	}
	n, e := Compare("1.2.3+abc", "1.2.3+def")
	if e != nil || n != 0 {
		t.Fatal(n, e)
	}
}
func TestRejectMalformedVersion(t *testing.T) {
	for _, s := range []string{"1.2", "v1.2.3", "01.2.3", "1.2.3-01", "1.2.3+", "1.2.3-a..b", "1.2.3\n", "-1.2.3"} {
		if _, e := Parse(s); e == nil {
			t.Fatal(s)
		}
	}
}
func TestStableIdentity(t *testing.T) {
	if (Identity{Version: "1.2.3", Channel: "stable", Commit: "unknown"}).Validate() == nil {
		t.Fatal("unknown stable accepted")
	}
	i := Identity{Version: "1.2.3", Channel: "dev", Commit: "12345678901234567890", Dirty: true}
	if i.Display() != "1.2.3-dev+g123456789012.dirty" {
		t.Fatal(i.Display())
	}
}
