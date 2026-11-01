package steam

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	sa := filepath.Join(root, "steamapps")
	for _, d := range []string{"common/Cool Game", "common/Proton 9.0", "compatdata/100", "shadercache/100"} {
		if err := os.MkdirAll(filepath.Join(sa, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(sa, "common/Cool Game/data.bin"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(sa, "libraryfolders.vdf"),
		[]byte(`"libraryfolders" { "0" { "path" "`+vdfPath(root)+`" } }`), 0o644)
	os.WriteFile(filepath.Join(sa, "appmanifest_100.acf"), []byte(`"AppState"
{
	"appid"		"100"
	"name"		"Cool Game"
	"installdir"		"Cool Game"
	"SizeOnDisk"		"5000"
}`), 0o644)
	os.WriteFile(filepath.Join(sa, "appmanifest_200.acf"), []byte(`"AppState"
{
	"appid"		"200"
	"name"		"Proton 9.0"
	"installdir"		"Proton 9.0"
	"SizeOnDisk"		"9000"
}`), 0o644)
	return root
}

func TestInstalled(t *testing.T) {
	root := fixture(t)
	games := Installed(Libraries(root))
	if len(games) != 2 {
		t.Fatalf("got %d games", len(games))
	}
	if games[0].Name != "Proton 9.0" || !games[0].Tool {
		t.Errorf("expected Proton first (largest) and flagged as tool: %+v", games[0])
	}
	if games[1].Name != "Cool Game" || games[1].Tool || games[1].Size != 5000 {
		t.Errorf("bad game: %+v", games[1])
	}
}

func TestUninstall(t *testing.T) {
	root := fixture(t)
	g := Installed(Libraries(root))[1]
	if err := Uninstall(g, UninstallOpts{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(g.gameDir()); err != nil {
		t.Fatal("dry run removed files")
	}
	if err := Uninstall(g, UninstallOpts{Prefix: true}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{g.gameDir(), g.manifest(), g.compatData(), g.shaderCache()} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("still exists: %s", p)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "steamapps/common/Proton 9.0")); err != nil {
		t.Error("removed the wrong game")
	}
}

func TestUninstallKeepsPrefixByDefault(t *testing.T) {
	root := fixture(t)
	g := Installed(Libraries(root))[1]
	if err := Uninstall(g, UninstallOpts{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(g.compatData()); err != nil {
		t.Error("compatdata removed without Prefix option")
	}
}

func TestUnsafeInstallDir(t *testing.T) {
	g := Game{AppID: "1", InstallDir: "../../etc", Library: t.TempDir()}
	if err := Uninstall(g, UninstallOpts{}); err == nil {
		t.Error("expected refusal")
	}
}

// vdfPath escapes backslashes the way Steam writes Windows paths in .vdf files.
func vdfPath(p string) string { return strings.ReplaceAll(p, `\`, `\\`) }
