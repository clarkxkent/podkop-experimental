package podkop

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The backend reaches sing-box through its package engine alone: no
// other package of it imports a package of sing-box, or sing-tun. A new version of
// podkop-engine, or the next line of sing-box, then has one package of the backend to port.
// The tests of a package may import what they build their cases with. The program
// (cmd/podkop) is where the backend and sing-box are put together: it runs the command
// line of sing-box under that name, and is not looked at.
func TestOneDoorToSingBox(t *testing.T) {
	const (
		singBox  = "github.com/sagernet/sing-box/"
		backend  = "github.com/podkop-net/podkop-experimental/src/backend"
		singTun  = "github.com/sagernet/sing-tun"
		theDoor  = "engine"
		ownFiles = ".go"
	)
	checked := 0
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path == theDoor || path == "cmd" || entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ownFiles) || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		checked++
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if imported == singTun || strings.HasPrefix(imported, singTun+"/") ||
				strings.HasPrefix(imported, singBox) && imported != backend && !strings.HasPrefix(imported, backend+"/") {
				t.Errorf("%s imports %s: only engine imports sing-box", path, imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 20 {
		t.Fatalf("only %d files were checked: the test does not look where the backend is", checked)
	}
}
