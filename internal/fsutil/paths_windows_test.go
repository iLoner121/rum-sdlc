package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsDriveRelativePreservesParentComponents(t *testing.T) {
	cwd, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	volume := filepath.VolumeName(cwd)
	path, e := AbsoluteInput(volume + `alias\..\destination`)
	if e != nil || !filepath.IsAbs(path) || !strings.Contains(path, `alias\..\destination`) {
		t.Fatal(path, e)
	}
	rooted, e := AbsoluteInput(`\destination`)
	if e != nil || rooted != volume+`\destination` {
		t.Fatal(rooted, e)
	}
}
