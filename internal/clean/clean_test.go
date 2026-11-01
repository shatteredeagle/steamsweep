package clean

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShatteredEagle/steamsweep/internal/steam"
)

func write(t *testing.T, p string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) (root string, games []steam.Game) {
	root = t.TempDir()
	sa := filepath.Join(root, "steamapps")
	write(t, filepath.Join(sa, "libraryfolders.vdf"), 0)
	os.WriteFile(filepath.Join(sa, "libraryfolders.vdf"), []byte(`"libraryfolders" { "0" { "path" "`+vdfPath(root)+`" } }`), 0o644)
	write(t, filepath.Join(sa, "common", "Kept", "a.bin"), 10)
	write(t, filepath.Join(sa, "common", "Gone", "a.bin"), 20)
	write(t, filepath.Join(sa, "common", "Proton 9.0", "a.bin"), 30)
	write(t, filepath.Join(sa, "shadercache", "100", "s"), 5)       // installed
	write(t, filepath.Join(sa, "shadercache", "999", "s"), 6)       // orphan
	write(t, filepath.Join(sa, "compatdata", "999", "pfx", "x"), 7) // orphan prefix
	write(t, filepath.Join(sa, "compatdata", "100", "pfx", "x"), 8) // installed prefix: must stay
	write(t, filepath.Join(sa, "compatdata", "3000000000", "x"), 9) // non-Steam shortcut: must stay
	write(t, filepath.Join(root, "logs", "a.txt"), 4)
	write(t, filepath.Join(root, "compatibilitytools.d", "GE-Proton9-1", "p"), 40)
	rec := filepath.Join(root, "userdata", "1", "gamerecordings")
	write(t, filepath.Join(rec, "video", "bg_100_20261006_130821", "v.m4s"), 100)
	write(t, filepath.Join(rec, "timelines", "timeline_10020261006_130821.json"), 3)
	return root, []steam.Game{{AppID: "100", Name: "Kept Game", InstallDir: "Kept", Library: root}}
}

func byName(items []Item) map[string]Item {
	m := map[string]Item{}
	for _, it := range items {
		m[it.Name] = it
	}
	return m
}

func TestScan(t *testing.T) {
	root, games := fixture(t)
	res := Scan(root, []string{root}, games)
	got := byName(res.Items)
	for _, want := range []string{"Gone", "App 999 (shader/workshop)", "App 999 Proton prefix", "Kept Game recording, 2026-10-06 13:08",
		"Shader caches of installed games", "Steam logs", "GE-Proton9-1"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing item %q; have %v", want, keys(got))
		}
	}
	for _, bad := range []string{"Kept", "Proton 9.0", "App 100 Proton prefix", "App 3000000000 Proton prefix"} {
		if _, ok := got[bad]; ok {
			t.Errorf("must not offer %q", bad)
		}
	}
	rec := got["Kept Game recording, 2026-10-06 13:08"]
	if len(rec.Paths) != 2 || rec.Size != 103 || rec.Safe {
		t.Errorf("recording item wrong: %+v", rec)
	}
	if got["App 999 Proton prefix"].Safe || got["GE-Proton9-1"].Safe {
		t.Error("prefix and proton items must not be in mark-all")
	}
}

func keys(m map[string]Item) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestMissingLibrarySkipsLeftovers(t *testing.T) {
	root, games := fixture(t)
	os.WriteFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"),
		[]byte(`"libraryfolders" { "0" { "path" "`+vdfPath(root)+`" } "1" { "path" "/nonexistent/mnt/games" } }`), 0o644)
	res := Scan(root, []string{root}, games)
	if len(res.Warnings) != 1 {
		t.Fatalf("want a warning, got %v", res.Warnings)
	}
	got := byName(res.Items)
	if _, ok := got["Gone"]; ok {
		t.Error("offered leftovers while a library is missing")
	}
	if _, ok := got["Steam logs"]; !ok {
		t.Error("caches should still be offered")
	}
}

func TestRemove(t *testing.T) {
	root, games := fixture(t)
	res := Scan(root, []string{root}, games)
	it := byName(res.Items)["Gone"]
	if err := Remove(it, root, []string{root}, true); err != nil || !exists(it.Paths[0]) {
		t.Fatal("dry run must not delete")
	}
	if err := Remove(it, root, []string{root}, false); err != nil || exists(it.Paths[0]) {
		t.Fatalf("not removed: %v", err)
	}
	if !exists(filepath.Join(root, "steamapps", "common", "Kept")) {
		t.Error("removed the wrong folder")
	}
	outside := Item{Paths: []string{t.TempDir()}}
	if err := Remove(outside, root, []string{root}, false); err == nil {
		t.Error("must refuse paths outside Steam")
	}
	if err := Remove(Item{Paths: []string{root}}, root, []string{root}, false); err == nil {
		t.Error("must refuse the Steam root itself")
	}
}

// vdfPath escapes backslashes the way Steam writes Windows paths in .vdf files.
func vdfPath(p string) string { return strings.ReplaceAll(p, `\`, `\\`) }

func TestOrphanNamesFromAppInfo(t *testing.T) {
	root, games := fixture(t)
	// minimal v40 appinfo (inline keys) naming app 999
	var kv []byte
	kv = append(kv, 0x00)
	kv = append(kv, []byte("appinfo\x00")...)
	kv = append(kv, 0x00)
	kv = append(kv, []byte("common\x00")...)
	kv = append(kv, 0x01)
	kv = append(kv, []byte("name\x00Lost Game\x00")...)
	kv = append(kv, 0x08, 0x08, 0x08)
	hdr := make([]byte, 4+4+8+20+4+20)
	entry := append([]byte{0xE7, 0x03, 0, 0}, 0, 0, 0, 0) // appid 999
	binary.LittleEndian.PutUint32(entry[4:], uint32(len(hdr)+len(kv)))
	file := []byte{0x28, 0x44, 0x56, 0x07, 1, 0, 0, 0}
	file = append(file, entry...)
	file = append(file, hdr...)
	file = append(file, kv...)
	file = append(file, 0, 0, 0, 0)
	os.MkdirAll(filepath.Join(root, "appcache"), 0o755)
	os.WriteFile(filepath.Join(root, "appcache", "appinfo.vdf"), file, 0o644)

	got := byName(Scan(root, []string{root}, games).Items)
	if _, ok := got["Lost Game Proton prefix"]; !ok {
		t.Errorf("orphan prefix not named from appinfo; have %v", keys(got))
	}
}
