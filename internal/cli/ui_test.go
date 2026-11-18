package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUIHelpAndInvalidFlagsDoNotInitializeDevice(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	home := filepath.Join(root, "absent")
	var out, stderr bytes.Buffer
	if e := Run(context.Background(), []string{"--home", home, "ui", "--help"}, strings.NewReader(""), &out, &stderr); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(stderr.String(), "no-open") {
		t.Fatal(stderr.String())
	}
	if _, e := os.Stat(home); !os.IsNotExist(e) {
		t.Fatal("help initialized device", e)
	}
	if e := Run(context.Background(), []string{"--home", home, "ui", "--port=-1", "--no-open"}, strings.NewReader(""), &out, &stderr); e == nil {
		t.Fatal("negative port accepted")
	}
	if _, e := os.Stat(home); !os.IsNotExist(e) {
		t.Fatal("invalid arguments initialized device", e)
	}
}
