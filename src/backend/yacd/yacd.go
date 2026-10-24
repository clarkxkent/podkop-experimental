// Package yacd keeps the dashboard YACD where sing-box serves it (the external_ui of the
// Clash API). The package of podkop-exp brings a small build of it (BuiltinDir); it is
// copied to a directory of its own (ServedDir) that sing-box serves and may write: the
// upgrade of the dashboard (POST /upgrade/ui of the Clash API) puts the dashboard of GitHub
// there, and the files of the package stay as they are. sing-box downloads the dashboard
// itself, synchronously within its start, only into an empty directory: the served
// directory is never left empty. An upgrade that fails leaves the dashboard served
// (podkop-engine r16 unpacks it into a directory of its own first).
//
// Which dashboard is served is told by a file beside the directory (ServedDir+".builtin"),
// which sing-box does not touch: the build of the package copied there and the hash of the
// index.html it had.
package yacd

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/podkop-net/podkop-experimental/src/backend/dataplane"
	E "github.com/sagernet/sing/common/exceptions"
)

var (
	// BuiltinDir holds the dashboard of the package, with Marker naming its build.
	BuiltinDir = dataplane.DataDir + "/yacd"
	// ServedDir is what sing-box serves (plan, external_ui).
	ServedDir = dataplane.DataDir + "/ui"
	// lockPath serializes the copies of the backend and of a program run beside it.
	lockPath = dataplane.RunDir + "/yacd.lock"
)

// Marker is the file of BuiltinDir that names the build of the package.
const Marker = ".podkop-builtin"

// record is the file beside the served directory: the build copied there, and the hash of
// its index.html.
func record(served string) string { return served + ".builtin" }

// access keeps the copies of one process apart; lock those of two.
var access sync.Mutex

func lock() func() {
	access.Lock()
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		// no lock file (no run directory, a test): the copies of this process are apart
		return access.Unlock
	}
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		access.Unlock()
	}
}

// Ensure gives the served directory the dashboard of the package when it has none, when it
// has another build of the package (by its name or by its index.html), and when it has one
// podkop-exp did not put there (sing-box downloaded it into an empty directory before
// podkop-exp brought one); a dashboard upgraded from GitHub over the copy (its index.html
// changed) is kept. It tells whether the directory has a dashboard now.
func Ensure(builtin string, served string) (bool, error) {
	defer lock()()
	build, err := os.ReadFile(filepath.Join(builtin, Marker))
	if err != nil {
		// no dashboard in the package: what is served, if anything, stays
		return hasFiles(served), nil
	}
	if hasFiles(served) {
		copied, hash, known := readRecord(served)
		switch {
		case !known:
			// not put there by podkop-exp: the dashboard of the package
		case hash != hashOf(filepath.Join(served, "index.html")):
			// upgraded from GitHub
			return true, nil
		case copied == strings.TrimSpace(string(build)) && hash == hashOf(filepath.Join(builtin, "index.html")):
			return true, nil
		}
	}
	err = restore(builtin, served)
	return hasFiles(served), err
}

// Restore puts the dashboard of the package into the served directory, whatever it held.
func Restore(builtin string, served string) error {
	defer lock()()
	return restore(builtin, served)
}

// restore makes the copy beside the served directory and puts it in its place at once:
// sing-box serves the one or the other, never half of one. The record goes first and comes
// back last: a copy cut short is no copy of the package, and the next Ensure makes it again.
func restore(builtin string, served string) error {
	build, err := os.ReadFile(filepath.Join(builtin, Marker))
	if err != nil || !hasFiles(builtin) {
		return E.New("the package has no dashboard in ", builtin)
	}
	next := served + ".new"
	previous := served + ".old"
	_ = os.RemoveAll(next)
	_ = os.RemoveAll(previous)
	err = copyTree(builtin, next)
	if err != nil {
		_ = os.RemoveAll(next)
		return E.Cause(err, "copy the dashboard")
	}
	err = os.Remove(record(served))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		_ = os.RemoveAll(next)
		return E.Cause(err, "remove the record of the dashboard")
	}
	err = os.Rename(served, previous)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		_ = os.RemoveAll(next)
		return E.Cause(err, "move the dashboard served")
	}
	err = os.Rename(next, served)
	if err != nil {
		_ = os.Rename(previous, served)
		_ = os.RemoveAll(next)
		return E.Cause(err, "put the dashboard in place")
	}
	content := strings.TrimSpace(string(build)) + "\n" + hashOf(filepath.Join(served, "index.html")) + "\n"
	err = os.WriteFile(record(served)+".new", []byte(content), 0o644)
	if err == nil {
		err = os.Rename(record(served)+".new", record(served))
	}
	_ = os.RemoveAll(previous)
	return err
}

// Remove deletes the served directory and what tells about it (the package is removed).
func Remove(served string) error {
	defer lock()()
	_ = os.Remove(record(served))
	return os.RemoveAll(served)
}

func readRecord(served string) (build string, hash string, known bool) {
	content, err := os.ReadFile(record(served))
	if err != nil {
		return "", "", false
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 || lines[1] == "" {
		return "", "", false
	}
	return lines[0], lines[1], true
}

func hashOf(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func hasFiles(directory string) bool {
	entries, err := os.ReadDir(directory)
	return err == nil && len(entries) > 0
}

func copyTree(from string, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()
		destination, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		_, err = io.Copy(destination, source)
		closeErr := destination.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}
