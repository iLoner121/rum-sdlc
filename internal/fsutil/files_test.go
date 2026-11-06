package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSymlinkCannotHideBeforeDotDot(t *testing.T) {
	base, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	outside := filepath.Join(base, "outside")
	os.Mkdir(outside, 0700)
	link := filepath.Join(base, "alias")
	if e = os.Symlink(outside, link); e != nil {
		t.Skipf("symlink unavailable: %v", e)
	}
	raw := link + string(filepath.Separator) + ".." + string(filepath.Separator) + "new"
	if _, e = RealPath(raw); e == nil {
		t.Fatal("symlink hidden by path normalization")
	}
	key := "HOME"
	if runtime.GOOS == "windows" {
		key = "USERPROFILE"
	}
	t.Setenv(key, base)
	if _, e = RealPath("~/alias/../new"); e == nil {
		t.Fatal("home expansion concealed a symlink")
	}
	ordinary := outside + string(filepath.Separator) + ".." + string(filepath.Separator) + "new"
	got, e := RealPath(ordinary)
	if e != nil || got != filepath.Join(base, "new") {
		t.Fatal(got, e)
	}
}

func TestCallerPathsExpandHomeAndCurrentDirectory(t *testing.T) {
	want, e := filepath.Abs("./client-relative")
	if e != nil {
		t.Fatal(e)
	}
	got, e := AbsoluteInput("./client-relative")
	if e != nil || got != want {
		t.Fatalf("%s != %s: %v", got, want, e)
	}
	base := t.TempDir()
	key := "HOME"
	if runtime.GOOS == "windows" {
		key = "USERPROFILE"
	}
	t.Setenv(key, base)
	got, e = AbsoluteInput("~/root")
	if e != nil || got != filepath.Join(base, "root") {
		t.Fatal(got, e)
	}
}
