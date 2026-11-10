package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMetaCommitFormatEscapingAndDeviceFallback(t *testing.T) {
	at := time.Date(2026, 11, 11, 0, 32, 10, 0, time.FixedZone("CST", 8*3600))
	got := metaCommitMessage(at, Object{"commit_name": "Name", "commit_email": "user@example.test"}, "Mac-mini")
	want := `2026-11-11T00:32:10+08:00 | user="Name <user@example.test>" | device="Mac-mini"`
	if got != want {
		t.Fatal(got)
	}
	escaped := metaCommitMessage(at, Object{"commit_name": "A\n\"B\" | C", "commit_email": "mail@example.test"}, "host\r\n\"quoted\"")
	if strings.ContainsAny(escaped, "\r\n") {
		t.Fatal("multiline message", escaped)
	}
	match := metaCommitPattern.FindStringSubmatch(escaped)
	if match == nil {
		t.Fatal(escaped)
	}
	if user, err := strconv.Unquote(match[2]); err != nil || user != "A\n\"B\" | C <mail@example.test>" {
		t.Fatal(user, err)
	}
	for _, test := range []struct{ host, id, want string }{{" host ", "id", "host"}, {"", "id", "id"}, {" \n", " id ", "id"}, {"", "", "unknown-device"}} {
		if value := metaCommitDevice(test.host, test.id); value != test.want {
			t.Fatal(value, test)
		}
	}
}

var metaCommitPattern = regexp.MustCompile(`^(\S+) \| user=("(?:\\.|[^"\\])*") \| device=("(?:\\.|[^"\\])*")$`)

func assertStandardMetaCommit(t *testing.T, root, ref, device string) {
	t.Helper()
	raw, err := exec.Command("git", "-C", root, "show", "-s", "--format=%B%x00%an%x00%ae%x00%cn%x00%ce%x00%aI%x00%cI", ref).Output()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\x00")
	if len(fields) != 7 {
		t.Fatal(string(raw))
	}
	message := strings.TrimSuffix(fields[0], "\n")
	match := metaCommitPattern.FindStringSubmatch(message)
	if match == nil {
		t.Fatal("nonstandard management message", message)
	}
	stamp, err := time.Parse(time.RFC3339, match[1])
	if err != nil {
		t.Fatal(err)
	}
	authorDate, ae := time.Parse(time.RFC3339, fields[5])
	committerDate, ce := time.Parse(time.RFC3339, fields[6])
	if ae != nil || ce != nil || !stamp.Equal(authorDate) || !stamp.Equal(committerDate) {
		t.Fatal(message, fields)
	}
	identity, err := strconv.Unquote(match[2])
	if err != nil {
		t.Fatal(err)
	}
	if identity != fmt.Sprintf("%s <%s>", fields[3], fields[4]) || fields[1] != fields[3] || fields[2] != fields[4] {
		t.Fatal("identity mismatch", fields)
	}
	actualDevice, err := strconv.Unquote(match[3])
	if err != nil || actualDevice != device {
		t.Fatal(actualDevice, device, err)
	}
}
func TestActualMetaCommitDatesIdentityAndNoop(t *testing.T) {
	t.Setenv("GIT_AUTHOR_DATE", "2001-01-01T00:00:00Z")
	t.Setenv("GIT_COMMITTER_DATE", "2002-01-01T00:00:00Z")
	d, _, _, root := setupDevices(t)
	record := create(t, d, root)
	result := call(t, d, "enable", Object{"project_id": "P-demo"})
	hostname, _ := os.Hostname()
	assertStandardMetaCommit(t, str(record["path"]), str(result["head"]), metaCommitDevice(hostname, ""))
	second := call(t, d, "sync", Object{"project_id": "P-demo"})
	if second["head"] != result["head"] {
		t.Fatal("no-op sync created commit")
	}
	if os.Getenv("GIT_AUTHOR_DATE") != "2001-01-01T00:00:00Z" || os.Getenv("GIT_COMMITTER_DATE") != "2002-01-01T00:00:00Z" {
		t.Fatal("parent environment changed")
	}
}
func TestDeviceIdentityInImportedStoreCommit(t *testing.T) {
	a, b, hub, root := setupDevices(t)
	create(t, a, root)
	call(t, a, "enable", Object{"project_id": "P-demo"})
	record := call(t, b, "import", Object{"repository": "alice/RumSDLC_Demo", "path": root + "/second"})
	doc(t, str(record["path"]), "from-second.md", "second-device\n")
	db, err := b.ReadDB()
	if err != nil {
		t.Fatal(err)
	}
	operation := &operation{device: b, ctx: context.Background(), db: db}
	engine := operation.engine(record, hub)
	engine.deviceName = "Device-B"
	result := engine.sync(record, hub.repos["alice/RumSDLC_Demo"])
	assertStandardMetaCommit(t, str(record["path"]), str(result["head"]), "Device-B")
}
