package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func metaCommitDevice(hostname, deviceID string) string {
	if name := strings.TrimSpace(hostname); name != "" {
		return name
	}
	if id := strings.TrimSpace(deviceID); id != "" {
		return id
	}
	return "unknown-device"
}
func metaCommitMessage(when time.Time, identity Object, device string) string {
	user := fmt.Sprintf("%s <%s>", str(identity["commit_name"]), str(identity["commit_email"]))
	return fmt.Sprintf("%s | user=%s | device=%s", when.Format(time.RFC3339), strconv.Quote(user), strconv.Quote(device))
}
