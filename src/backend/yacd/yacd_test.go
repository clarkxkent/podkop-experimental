package yacd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func write(t *testing.T, path string, content string) {
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func read(t *testing.T, path string) string {
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

// The dashboard of the package goes where sing-box serves it when nothing is there, when
// an older build of the package is, and when a dashboard is there podkop-exp did not put
// (sing-box downloaded it into an empty directory); one upgraded from GitHub over the copy
// stays until it is restored; a package without a dashboard leaves what is served alone.
func TestEnsure(t *testing.T) {
	root := t.TempDir()
	builtin, served := filepath.Join(root, "yacd"), filepath.Join(root, "ui")
	write(t, filepath.Join(builtin, Marker), "build 1\n")
	write(t, filepath.Join(builtin, "index.html"), "<html>1</html>")
	write(t, filepath.Join(builtin, "assets", "index.js"), "one")

	ok, err := Ensure(builtin, served)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "<html>1</html>", read(t, filepath.Join(served, "index.html")))
	require.Equal(t, "one", read(t, filepath.Join(served, "assets", "index.js")))
	require.Contains(t, read(t, served+".builtin"), "build 1\n")

	// the same build: nothing is copied (an asset of it changed is left)
	write(t, filepath.Join(served, "assets", "index.js"), "touched")
	ok, err = Ensure(builtin, served)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "touched", read(t, filepath.Join(served, "assets", "index.js")))

	// the same name of the build with another index.html in the package (rebuilt under the
	// same name): replaced
	write(t, filepath.Join(builtin, "index.html"), "<html>1b</html>")
	_, err = Ensure(builtin, served)
	require.NoError(t, err)
	require.Equal(t, "<html>1b</html>", read(t, filepath.Join(served, "index.html")))
	require.Equal(t, "one", read(t, filepath.Join(served, "assets", "index.js")))
	write(t, filepath.Join(builtin, "index.html"), "<html>1</html>")

	// a newer package: its build replaces the older one, files of the old one go
	write(t, filepath.Join(served, "stale.js"), "old")
	write(t, filepath.Join(builtin, Marker), "build 2\n")
	ok, err = Ensure(builtin, served)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "one", read(t, filepath.Join(served, "assets", "index.js")))
	require.NoFileExists(t, filepath.Join(served, "stale.js"))
	require.NoDirExists(t, served+".old")
	require.NoDirExists(t, served+".new")

	// upgraded from GitHub: sing-box unpacks another dashboard over the copy, the files it
	// does not overwrite (the marker of the package among them) stay
	write(t, filepath.Join(served, Marker), "build 2\n")
	write(t, filepath.Join(served, "index.html"), "<html>github</html>")
	write(t, filepath.Join(served, "assets", "github.js"), "two")
	for range 2 {
		ok, err = Ensure(builtin, served)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "<html>github</html>", read(t, filepath.Join(served, "index.html")))
	}
	// a newer package keeps it too
	write(t, filepath.Join(builtin, Marker), "build 3\n")
	_, err = Ensure(builtin, served)
	require.NoError(t, err)
	require.Equal(t, "<html>github</html>", read(t, filepath.Join(served, "index.html")))
	// until the dashboard of the package is asked back
	require.NoError(t, Restore(builtin, served))
	require.Equal(t, "<html>1</html>", read(t, filepath.Join(served, "index.html")))
	require.NoFileExists(t, filepath.Join(served, "assets", "github.js"))
	require.Contains(t, read(t, served+".builtin"), "build 3\n")

	// a dashboard podkop-exp did not put there (sing-box downloaded it before): replaced
	require.NoError(t, os.Remove(served+".builtin"))
	write(t, filepath.Join(served, "index.html"), "<html>downloaded</html>")
	_, err = Ensure(builtin, served)
	require.NoError(t, err)
	require.Equal(t, "<html>1</html>", read(t, filepath.Join(served, "index.html")))

	// a failed upgrade empties the directory: the copy is made again
	require.NoError(t, os.RemoveAll(served))
	require.NoError(t, os.MkdirAll(served, 0o755))
	ok, err = Ensure(builtin, served)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "<html>1</html>", read(t, filepath.Join(served, "index.html")))

	// a package without a dashboard: what is served stays, an empty directory is no dashboard
	empty := filepath.Join(root, "none")
	ok, err = Ensure(empty, served)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = Ensure(empty, filepath.Join(root, "nothing"))
	require.NoError(t, err)
	require.False(t, ok)
	require.Error(t, Restore(empty, served))
	require.Equal(t, "<html>1</html>", read(t, filepath.Join(served, "index.html")), "a failed restore leaves what is served")

	require.NoError(t, Remove(served))
	require.NoDirExists(t, served)
	require.NoFileExists(t, served+".builtin")
}

// One upgrade from GitHub at a time, in the background; its end is told by Status.
func TestUpgrade(t *testing.T) {
	var upgrade Upgrade
	release := make(chan error)
	require.True(t, upgrade.Start(func() error { return <-release }))
	require.False(t, upgrade.Start(func() error { return nil }), "one at a time")
	running, last := upgrade.Status()
	require.True(t, running)
	require.False(t, last.Done)
	release <- errors.New("no network")
	ended := func() bool {
		running, _ := upgrade.Status()
		return !running
	}
	require.Eventually(t, ended, 5*time.Second, 10*time.Millisecond)
	_, last = upgrade.Status()
	require.Equal(t, UpgradeResult{Done: true, Message: "no network", At: last.At}, last)
	require.NotZero(t, last.At)
	require.True(t, upgrade.Start(func() error { return nil }))
	require.Eventually(t, ended, 5*time.Second, 10*time.Millisecond)
	_, last = upgrade.Status()
	require.True(t, last.Success)
}
