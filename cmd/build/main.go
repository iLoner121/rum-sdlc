// build creates self-contained native binaries for desktop/server platforms.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"rumsdlc/internal/core"
	"rumsdlc/internal/release"
	"runtime"
	"sort"
	"strings"
	"time"
)

func build() error {
	targets := []struct{ OS, Arch string }{{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}}
	directory := filepath.Join("build", "native", release.Version)
	if e := os.MkdirAll(directory, 0755); e != nil {
		return e
	}
	goTool := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goTool += ".exe"
	}
	checksums := map[string]string{}
	for _, target := range targets {
		name := "rum-" + target.OS + "-" + target.Arch
		if target.OS == "windows" {
			name += ".exe"
		}
		path := filepath.Join(directory, name)
		command := exec.Command(goTool, "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w -X rumsdlc/internal/release.Channel="+release.Channel+" -X rumsdlc/internal/release.Commit="+release.Commit+" -X rumsdlc/internal/release.Dirty="+release.Dirty+" -X rumsdlc/internal/release.BuiltAt="+release.BuiltAt, "-o", path, "./cmd/rum")
		command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target.OS, "GOARCH="+target.Arch)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if e := command.Run(); e != nil {
			return fmt.Errorf("%s: %w", name, e)
		}
		raw, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		hash := sha256.Sum256(raw)
		checksums[name] = hex.EncodeToString(hash[:])
		fmt.Println(name)
		archive := filepath.Join(directory, "rumsdlc-"+release.Version+"-"+target.OS+"-"+target.Arch)
		if target.OS == "windows" {
			archive += ".zip"
			e = zipBinary(archive, "rum.exe", raw)
		} else {
			archive += ".tar.gz"
			e = tarBinary(archive, "rum", raw)
		}
		if e != nil {
			return e
		}
		data, e := os.ReadFile(archive)
		if e != nil {
			return e
		}
		hash = sha256.Sum256(data)
		checksums[filepath.Base(archive)] = hex.EncodeToString(hash[:])
	}
	sourceName := "rumsdlc-" + release.Version + "-source.zip"
	if e := sourceArchive(filepath.Join(directory, sourceName)); e != nil {
		return e
	}
	source, e := os.ReadFile(filepath.Join(directory, sourceName))
	if e != nil {
		return e
	}
	sourceHash := sha256.Sum256(source)
	checksums[sourceName] = hex.EncodeToString(sourceHash[:])
	manifest, e := json.MarshalIndent(map[string]any{"format": "rum-release/1", "version": release.Version, "source_commit": release.Commit, "channel": release.Channel, "platforms": targets}, "", "  ")
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(directory, "release-manifest.json"), append(manifest, '\n'), 0644); e != nil {
		return e
	}
	mh := sha256.Sum256(append(manifest, '\n'))
	checksums["release-manifest.json"] = hex.EncodeToString(mh[:])
	data, e := json.MarshalIndent(map[string]any{"version": release.Version, "source_commit": release.Commit, "channel": release.Channel, "implementation": "go-native", "go_version": runtime.Version(), "runtime_dependencies": []string{"Git for repository operations; project scripts keep their Python/tool dependencies"}, "acceptance": "See ACCEPTANCE.md for execution evidence; cross-compilation alone is not runtime acceptance", "files": checksums}, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(directory, "checksums.json"), append(data, '\n'), 0644)
}

// Source and executable packages share one release identity and checksum index.
// The source manifest records the exact code, docs and acceptance evidence.
func sourceArchive(path string) error {
	names := []string{"go.mod", ".gitignore", ".gitattributes", "README.md", "ACCEPTANCE.md", "CHANGELOG.md", "release-please-config.json", ".release-please-manifest.json"}
	for _, folder := range []string{"cmd", "internal", "tests", ".github", "evidence", "installer-skill", "docs"} {
		err := filepath.WalkDir(folder, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == "__pycache__" {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("Source archive cannot contain symlinks: %s", name)
			}
			names = append(names, name)
			return nil
		})
		if err != nil {
			return err
		}
	}
	sort.Strings(names)
	file, e := os.Create(path)
	if e != nil {
		return e
	}
	defer file.Close()
	archive := zip.NewWriter(file)
	defer archive.Close()
	write := func(name string, raw []byte) error {
		header := &zip.FileHeader{Name: "rum-sdlc-device-v" + release.Version + "/" + filepath.ToSlash(name), Method: zip.Deflate}
		header.SetMode(0644)
		header.SetModTime(time.Unix(0, 0).UTC())
		entry, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = entry.Write(raw)
		return err
	}
	hashes := map[string]string{}
	for _, name := range names {
		raw, e := os.ReadFile(name)
		if e != nil {
			return e
		}
		if e = write(name, raw); e != nil {
			return e
		}
		hash := sha256.Sum256(raw)
		hashes[filepath.ToSlash(name)] = hex.EncodeToString(hash[:])
	}
	manifest, e := json.MarshalIndent(map[string]any{"name": "rum-sdlc-go", "version": release.Version, "core_version": core.Version, "implementation": "go-native", "files": hashes}, "", "  ")
	if e != nil {
		return e
	}
	if e = write("MANIFEST.json", append(manifest, '\n')); e != nil {
		return e
	}
	return archive.Close()
}

func zipBinary(path, name string, raw []byte) error {
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer f.Close()
	w := zip.NewWriter(f)
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0755)
	header.SetModTime(time.Unix(0, 0).UTC())
	entry, e := w.CreateHeader(header)
	if e != nil {
		return e
	}
	if _, e = entry.Write(raw); e != nil {
		return e
	}
	if e = w.Close(); e != nil {
		return e
	}
	return f.Close()
}
func tarBinary(path, name string, raw []byte) error {
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer f.Close()
	gzipWriter := gzip.NewWriter(f)
	tarWriter := tar.NewWriter(gzipWriter)
	if e = tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(raw)), ModTime: time.Unix(0, 0)}); e != nil {
		return e
	}
	if _, e = tarWriter.Write(raw); e != nil {
		return e
	}
	if e = tarWriter.Close(); e != nil {
		return e
	}
	if e = gzipWriter.Close(); e != nil {
		return e
	}
	return f.Close()
}
func main() {
	channel := flag.String("channel", "dev", "dev or stable")
	commit := flag.String("commit", "", "exact source SHA")
	flag.Parse()
	rev, e := exec.Command("git", "rev-parse", "HEAD").Output()
	if e == nil {
		release.Commit = strings.TrimSpace(string(rev))
	} else {
		release.Commit = "unknown"
	}
	dirty, e := exec.Command("git", "status", "--porcelain").Output()
	release.Dirty = "false"
	if e != nil || len(dirty) > 0 {
		release.Dirty = "true"
	}
	release.Channel = *channel
	if *channel != "dev" && *channel != "stable" {
		fmt.Fprintln(os.Stderr, "invalid build channel")
		os.Exit(1)
	}
	if *commit != "" && *commit != release.Commit {
		fmt.Fprintln(os.Stderr, "source SHA differs from requested commit")
		os.Exit(1)
	}
	if *channel == "stable" && (*commit == "" || release.Dirty != "false" || release.Current().Validate() != nil) {
		fmt.Fprintln(os.Stderr, "stable build requires a clean fixed SHA")
		os.Exit(1)
	}
	date, e := exec.Command("git", "show", "-s", "--format=%cI", "HEAD").Output()
	if e == nil {
		release.BuiltAt = strings.TrimSpace(string(date))
	}
	if e := build(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
