// Package steam discovers Steam libraries and installed games, and uninstalls them.
package steam

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/andygrunwald/vdf"
)

// Game is one installed Steam app, read from an appmanifest_*.acf file.
type Game struct {
	AppID       string
	Name        string
	InstallDir  string
	Size        int64  // SizeOnDisk from the manifest, bytes
	LastPlayed  int64  // unix seconds, 0 = never
	LastUpdated int64  // unix seconds
	Library     string // library root (contains steamapps/)
	Tool        bool   // Proton, Steam Linux Runtime, redistributables
}

func (g Game) steamapps() string  { return filepath.Join(g.Library, "steamapps") }
func (g Game) manifest() string   { return filepath.Join(g.steamapps(), "appmanifest_"+g.AppID+".acf") }
func (g Game) gameDir() string    { return filepath.Join(g.steamapps(), "common", g.InstallDir) }
func (g Game) compatData() string { return filepath.Join(g.steamapps(), "compatdata", g.AppID) }
func (g Game) shaderCache() string {
	return filepath.Join(g.steamapps(), "shadercache", g.AppID)
}

var toolAppIDs = map[string]bool{
	"228980":  true, // Steamworks Common Redistributables
	"1070560": true, // Steam Linux Runtime 1.0 (scout)
	"1391110": true, // Steam Linux Runtime 2.0 (soldier)
	"1628350": true, // Steam Linux Runtime 3.0 (sniper)
	"1493710": true, // Proton Experimental
}

func isTool(appid, name string) bool {
	return toolAppIDs[appid] ||
		strings.HasPrefix(name, "Proton ") ||
		strings.HasPrefix(name, "Steam Linux Runtime") ||
		strings.HasPrefix(name, "Steamworks")
}

// candidateRoots lists the usual Steam install locations for the current OS.
func candidateRoots() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		var out []string
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
			if base := os.Getenv(env); base != "" {
				out = append(out, filepath.Join(base, "Steam"))
			}
		}
		return append(out, `C:\Program Files (x86)\Steam`)
	case "darwin":
		return []string{filepath.Join(home, "Library", "Application Support", "Steam")}
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	return []string{
		filepath.Join(data, "Steam"),
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam"), // Flatpak
		filepath.Join(home, "snap", "steam", "common", ".local", "share", "Steam"),     // Snap
	}
}

// FindRoot returns the Steam root, trying override, $STEAM_ROOT, then the usual places.
func FindRoot(override string) (string, error) {
	var order []string
	if override != "" {
		order = append(order, override)
	}
	if env := os.Getenv("STEAM_ROOT"); env != "" {
		order = append(order, env)
	}
	order = append(order, candidateRoots()...)
	for _, p := range order {
		if fi, err := os.Stat(filepath.Join(p, "steamapps")); err == nil && fi.IsDir() {
			if real, err := filepath.EvalSymlinks(p); err == nil {
				return real, nil
			}
			return p, nil
		}
	}
	return "", errors.New("Steam not found; pass --steam-root or set STEAM_ROOT")
}

// Running reports whether the Steam client is running (it rewrites manifests while open).
func Running() bool {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq steam.exe", "/NH").Output()
		return err == nil && strings.Contains(strings.ToLower(string(out)), "steam.exe")
	case "darwin":
		return exec.Command("pgrep", "-x", "steam_osx").Run() == nil
	}
	return exec.Command("pgrep", "-x", "steam").Run() == nil
}

func parseVDF(path string) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return vdf.NewParser(f).Parse()
}

func asMap(v any) map[string]any { m, _ := v.(map[string]any); return m }
func asStr(v any) string         { s, _ := v.(string); return s }

// Libraries returns every library root listed in libraryfolders.vdf, plus root itself.
func Libraries(root string) []string {
	seen := map[string]bool{}
	var libs []string
	add := func(p string) {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		if p == "" || seen[p] {
			return
		}
		if fi, err := os.Stat(filepath.Join(p, "steamapps")); err != nil || !fi.IsDir() {
			return
		}
		seen[p] = true
		libs = append(libs, p)
	}
	add(root)
	if m, err := parseVDF(filepath.Join(root, "steamapps", "libraryfolders.vdf")); err == nil {
		entries := asMap(m["libraryfolders"])
		keys := make([]string, 0, len(entries))
		for k := range entries {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			add(asStr(asMap(entries[k])["path"]))
		}
	}
	return libs
}

// Installed lists installed games across all libraries, largest first.
func Installed(libs []string) []Game {
	var games []Game
	for _, lib := range libs {
		matches, _ := filepath.Glob(filepath.Join(lib, "steamapps", "appmanifest_*.acf"))
		for _, mf := range matches {
			parsed, err := parseVDF(mf)
			if err != nil {
				continue
			}
			st := asMap(parsed["AppState"])
			appid := asStr(st["appid"])
			if appid == "" {
				continue
			}
			size, _ := strconv.ParseInt(asStr(st["SizeOnDisk"]), 10, 64)
			played, _ := strconv.ParseInt(asStr(st["LastPlayed"]), 10, 64)
			updated, _ := strconv.ParseInt(asStr(st["LastUpdated"]), 10, 64)
			name := asStr(st["name"])
			if name == "" {
				name = "App " + appid
			}
			games = append(games, Game{
				AppID: appid, Name: name, InstallDir: asStr(st["installdir"]),
				Size: size, LastPlayed: played, LastUpdated: updated, Library: lib, Tool: isTool(appid, name),
			})
		}
	}
	sort.SliceStable(games, func(i, j int) bool { return games[i].Size > games[j].Size })
	return games
}

// UninstallOpts controls what Uninstall removes beyond the game files.
type UninstallOpts struct {
	Prefix bool // also remove compatdata (Proton prefix, may hold saves) and shadercache
	DryRun bool
}

// Uninstall removes the game's install folder, then its manifest. The manifest goes last so a
// failed removal leaves the game visible to Steam instead of half-gone.
func Uninstall(g Game, o UninstallOpts) error {
	if g.AppID == "" || strings.ContainsAny(g.AppID, `/\`) {
		return fmt.Errorf("bad appid %q", g.AppID)
	}
	if o.DryRun {
		return nil
	}
	if g.InstallDir != "" {
		clean := filepath.Base(g.InstallDir)
		if clean != g.InstallDir || clean == "." || clean == ".." || clean == string(filepath.Separator) {
			return fmt.Errorf("refusing unsafe installdir %q", g.InstallDir)
		}
		if err := os.RemoveAll(g.gameDir()); err != nil {
			return err
		}
	}
	if o.Prefix {
		for _, p := range []string{g.compatData(), g.shaderCache()} {
			if err := os.RemoveAll(p); err != nil {
				return err
			}
		}
	}
	if err := os.Remove(g.manifest()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// getFold is a case-insensitive map lookup; localconfig.vdf capitalisation varies between clients.
func getFold(m map[string]any, key string) map[string]any {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return asMap(v)
		}
	}
	return nil
}

// ApplyLastPlayed raises each game's LastPlayed to the newest value found in any Steam user's
// localconfig.vdf, which is more complete than the appmanifest field.
func ApplyLastPlayed(root string, games []Game) {
	files, _ := filepath.Glob(filepath.Join(root, "userdata", "*", "config", "localconfig.vdf"))
	best := map[string]int64{}
	for _, f := range files {
		parsed, err := parseVDF(f)
		if err != nil {
			continue
		}
		apps := getFold(getFold(getFold(getFold(parsed, "UserLocalConfigStore"), "Software"), "Valve"), "Steam")
		apps = getFold(apps, "apps")
		for id, v := range apps {
			if t, _ := strconv.ParseInt(asStr(asMap(v)["LastPlayed"]), 10, 64); t > best[id] {
				best[id] = t
			}
		}
	}
	for i := range games {
		if t := best[games[i].AppID]; t > games[i].LastPlayed {
			games[i].LastPlayed = t
		}
	}
}

// MissingLibraries returns library paths that libraryfolders.vdf lists but that are not on disk
// right now (typically an unmounted drive). Games installed there look uninstalled.
func MissingLibraries(root string) []string {
	m, err := parseVDF(filepath.Join(root, "steamapps", "libraryfolders.vdf"))
	if err != nil {
		return nil
	}
	var missing []string
	for _, v := range asMap(m["libraryfolders"]) {
		p := asStr(asMap(v)["path"])
		if p == "" {
			continue
		}
		if fi, err := os.Stat(filepath.Join(p, "steamapps")); err != nil || !fi.IsDir() {
			missing = append(missing, p)
		}
	}
	sort.Strings(missing)
	return missing
}

// IsTool reports whether an appid is a known runtime/tool rather than a game.
func IsToolAppID(appid string) bool { return toolAppIDs[appid] }
