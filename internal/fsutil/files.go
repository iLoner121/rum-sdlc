// Package fsutil contains the device's filesystem and interprocess primitives.
package fsutil

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/store"
	"strings"
	"time"
)

func ID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	return hex.EncodeToString(b[:]), nil
}
func Now() string             { return time.Now().UTC().Format("2006-01-02T15:04:05.000000+00:00") }
func Exists(path string) bool { _, e := os.Stat(path); return e == nil }
func IsDir(path string) bool  { i, e := os.Stat(path); return e == nil && i.IsDir() }
func IsFile(path string) bool { i, e := os.Stat(path); return e == nil && i.Mode().IsRegular() }

// AbsoluteInput preserves dot components until the receiving application's
// symlink checks. CLI callers use it before sending paths over local IPC.
func AbsoluteInput(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") || (os.PathSeparator == '\\' && strings.HasPrefix(path, "~\\")) {
		home, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		path = home + path[1:]
	}
	// Removing current-directory components is safe; resolving '..' here could
	// conceal a symlink that the receiving application must reject.
	parts := strings.Split(filepath.FromSlash(path), string(filepath.Separator))
	kept := parts[:0]
	for _, part := range parts {
		if part != "." {
			kept = append(kept, part)
		}
	}
	path = strings.Join(kept, string(filepath.Separator))
	if filepath.IsAbs(path) {
		return path, nil
	}
	cwd, e := os.Getwd()
	if e != nil {
		return "", e
	}
	if os.PathSeparator == '\\' {
		if strings.HasPrefix(path, `\`) {
			return filepath.VolumeName(cwd) + path, nil
		}
		if volume := filepath.VolumeName(path); volume != "" {
			// Resolve the drive's current directory without handing '..' from the
			// caller to filepath.Abs, which would erase it before symlink checks.
			anchor, err := filepath.Abs(volume + ".")
			if err != nil {
				return "", err
			}
			return anchor + string(filepath.Separator) + strings.TrimPrefix(path, volume), nil
		}
	}
	if path == "" {
		return cwd, nil
	}
	return cwd + string(filepath.Separator) + path, nil
}
func RealPath(path string) (string, error) {
	original, e := AbsoluteInput(path)
	if e != nil {
		return "", e
	}
	// Inspect each original component before cleaning '..'; otherwise a symlink
	// immediately before '..' could be hidden by lexical normalization.
	volume := filepath.VolumeName(original)
	prefix := volume
	rest := strings.TrimPrefix(original, volume)
	if strings.HasPrefix(rest, string(filepath.Separator)) || strings.HasPrefix(rest, "/") {
		prefix += string(filepath.Separator)
	}
	for _, part := range strings.FieldsFunc(rest, func(r rune) bool { return r == rune(filepath.Separator) || r == '/' }) {
		if prefix != "" && !strings.HasSuffix(prefix, string(filepath.Separator)) {
			prefix += string(filepath.Separator)
		}
		prefix += part
		info, err := os.Lstat(prefix)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("Use a real directory, not a symlink")
		}
	}
	absolute := filepath.Clean(original)
	for p := absolute; ; p = filepath.Dir(p) {
		i, e := os.Lstat(p)
		if e != nil && !os.IsNotExist(e) {
			return "", e
		}
		if e == nil && i.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("Use a real directory, not a symlink")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return absolute, nil
}
func Contains(parent, child string) bool {
	rel, e := filepath.Rel(parent, child)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func Overlap(a, b string) bool { return Contains(a, b) || Contains(b, a) }
func ReadJSON(path string) (jsonvalue.Object, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	v, e := jsonvalue.Decode(b)
	if e != nil {
		return nil, fmt.Errorf("%s: %w", path, e)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must contain a JSON object", path)
	}
	return m, nil
}
func WriteJSON(path string, value any) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	return store.Atomic(path, value)
}

type Lock struct{ file *os.File }

func Acquire(path string, exclusive, blocking bool) (*Lock, error) {
	if i, e := os.Lstat(path); e == nil && i.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("Runtime gate cannot be a symlink")
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = lockFile(f, exclusive, blocking); e != nil {
		f.Close()
		return nil, e
	}
	return &Lock{file: f}, nil
}
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	e := unlockFile(l.file)
	closeErr := l.file.Close()
	l.file = nil
	if e != nil {
		return e
	}
	return closeErr
}
func Lease(home string, exclusive bool) (*Lock, error) {
	if e := os.MkdirAll(home, 0700); e != nil {
		return nil, e
	}
	lock, e := Acquire(filepath.Join(home, "install.lock"), exclusive, false)
	if e != nil {
		if exclusive {
			return nil, fmt.Errorf("RumSDLC is running; close commands and stop the service before upgrading: %w", e)
		}
		return nil, fmt.Errorf("RumSDLC installation is in progress; retry after it finishes: %w", e)
	}
	if !exclusive && Exists(filepath.Join(home, "install-transaction.json")) {
		lock.Close()
		return nil, fmt.Errorf("Pending installation recovery; run installer --home %s --recover-only", home)
	}
	return lock, nil
}

// WriteBytes replaces one generated resource atomically after syncing its data.
func WriteBytes(path string, data []byte) (err error) {
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { f.Close(); os.Remove(name) }()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// VerifyNativeRelease checks the immutable release payload before a stable
// launcher executes it. The manifest is generated by the offline installer.
func VerifyNativeRelease(executable string) error {
	path, e := RealPath(executable)
	if e != nil {
		return e
	}
	manifest, e := ReadJSON(filepath.Join(filepath.Dir(path), "MANIFEST.json"))
	if e != nil {
		return e
	}
	if manifest["name"] != "rum-sdlc-native" {
		return fmt.Errorf("Invalid native release manifest")
	}
	files, ok := manifest["files"].(map[string]any)
	if !ok || len(files) != 1 {
		return fmt.Errorf("Invalid native release inventory")
	}
	expected, ok := files[filepath.Base(path)].(string)
	if !ok || len(expected) != 64 {
		return fmt.Errorf("Native runtime is absent from its release manifest")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if jsonvalue.HashBytes(raw) != expected {
		return fmt.Errorf("Native runtime differs from its immutable release manifest")
	}
	return nil
}
