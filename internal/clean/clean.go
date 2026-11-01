// Package clean finds Steam clutter that is not an installed game: recordings, caches,
// leftovers from uninstalled games, and old Proton builds.
package clean

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ShatteredEagle/steamsweep/internal/steam"
)

type Kind string

const (
	KindRecording Kind = "recording"
	KindLeftover  Kind = "leftover"
	KindPrefix    Kind = "prefix"
	KindCache     Kind = "cache"
	KindProton    Kind = "proton"
	KindDownload  Kind = "partial"
)

// Item is one thing the user can mark for deletion. It may span several paths.
type Item struct {
	Key   string
	Name  string
	Kind  Kind
	Paths []string
	Size  int64
	Safe  bool // included by "mark all"; false for anything that may hold personal data or progress
}

// Result is what a scan found, plus reasons it skipped anything.
type Result struct {
	Items    []Item
	Warnings []string
}

func pathSize(p string) int64 {
	var total int64
	_ = filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func sum(paths []string) (n int64) {
	for _, p := range paths {
		n += pathSize(p)
	}
	return
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func dirs(path string) []string {
	entries, _ := os.ReadDir(path)
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func appName(names map[string]string, id string) string {
	if n := names[id]; n != "" {
		return n
	}
	return "App " + id
}

// nonSteamID reports ids that are not Steam games: Non-Steam shortcuts (32-bit values with the high
// bit set), the placeholder 0, and anything non-numeric.
func nonSteamID(id string) bool {
	n, err := strconv.ParseUint(id, 10, 64)
	return err != nil || n == 0 || n >= 1<<31 // 0 is Steam's own placeholder, not a game
}

var serviceDirs = []string{"Proton", "SteamLinuxRuntime", "Steamworks Shared", "Steam Controller Configs"}

func isServiceDir(name string) bool {
	for _, p := range serviceDirs {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return name == "Steam.dll"
}

// Scan inspects the Steam root and libraries. games must be every installed app, tools included.
func Scan(root string, libs []string, games []steam.Game) Result {
	var res Result
	names := map[string]string{}
	installed := map[string]bool{}
	installDirs := map[string]bool{}
	for _, g := range games {
		names[g.AppID] = g.Name
		installed[g.AppID] = true
		if g.InstallDir != "" {
			installDirs[g.InstallDir] = true
		}
	}
	add := func(it Item) {
		if len(it.Paths) == 0 {
			return
		}
		it.Size = sum(it.Paths)
		if it.Size == 0 {
			return
		}
		res.Items = append(res.Items, it)
	}

	// Names of uninstalled games come from Steam's local app database.
	unknown := map[string]bool{}
	for _, lib := range libs {
		sa := filepath.Join(lib, "steamapps")
		for _, sub := range []string{"compatdata", "shadercache", filepath.Join("workshop", "content")} {
			for _, d := range dirs(filepath.Join(sa, sub)) {
				if !installed[d] {
					unknown[d] = true
				}
			}
		}
	}
	for _, id := range recordingAppIDs(root) {
		if !installed[id] {
			unknown[id] = true
		}
	}
	for id, name := range steam.AppNames(root, unknown) {
		names[id] = name
	}

	missing := steam.MissingLibraries(root)
	orphansOK := len(missing) == 0
	if !orphansOK {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"Library not found (%s): leftover detection is off, since its games would look uninstalled.", strings.Join(missing, ", ")))
	}

	for _, lib := range libs {
		sa := filepath.Join(lib, "steamapps")
		if orphansOK {
			for _, d := range dirs(filepath.Join(sa, "common")) {
				if installDirs[d] || isServiceDir(d) {
					continue
				}
				add(Item{Key: "common:" + lib + ":" + d, Name: d, Kind: KindLeftover, Safe: true,
					Paths: []string{filepath.Join(sa, "common", d)}})
			}
			ids := map[string]bool{}
			for _, sub := range []string{"compatdata", "shadercache", filepath.Join("workshop", "content")} {
				for _, d := range dirs(filepath.Join(sa, sub)) {
					ids[d] = true
				}
			}
			var sorted []string
			for id := range ids {
				sorted = append(sorted, id)
			}
			sort.Strings(sorted)
			for _, id := range sorted {
				if installed[id] || steam.IsToolAppID(id) || nonSteamID(id) {
					continue
				}
				var junk []string
				for _, p := range []string{
					filepath.Join(sa, "shadercache", id),
					filepath.Join(sa, "workshop", "content", id),
					filepath.Join(sa, "workshop", "appworkshop_"+id+".acf"),
				} {
					if exists(p) {
						junk = append(junk, p)
					}
				}
				if len(junk) > 0 {
					add(Item{Key: "orphan:" + lib + ":" + id, Name: appName(names, id) + " (shader/workshop)", Kind: KindLeftover, Safe: true, Paths: junk})
				}
				if pfx := filepath.Join(sa, "compatdata", id); exists(pfx) {
					add(Item{Key: "prefix:" + lib + ":" + id, Name: appName(names, id) + " Proton prefix", Kind: KindPrefix, Paths: []string{pfx}})
				}
			}
		}
		var shaders []string
		for _, d := range dirs(filepath.Join(sa, "shadercache")) {
			if installed[d] {
				shaders = append(shaders, filepath.Join(sa, "shadercache", d))
			}
		}
		add(Item{Key: "shaders:" + lib, Name: "Shader caches of installed games", Kind: KindCache, Safe: true, Paths: shaders})

		var partial []string
		for _, sub := range []string{"downloading", "temp"} {
			entries, _ := os.ReadDir(filepath.Join(sa, sub))
			for _, e := range entries {
				partial = append(partial, filepath.Join(sa, sub, e.Name()))
			}
		}
		add(Item{Key: "partial:" + lib, Name: "Incomplete downloads", Kind: KindDownload, Paths: partial})
	}

	cacheDir := func(key, name, rel string) {
		if p := filepath.Join(root, rel); exists(p) {
			add(Item{Key: key, Name: name, Kind: KindCache, Safe: true, Paths: []string{p}})
		}
	}
	cacheDir("htmlcache", "Steam web cache", filepath.Join("config", "htmlcache"))
	cacheDir("httpcache", "Store/HTTP cache", filepath.Join("appcache", "httpcache"))
	cacheDir("librarycache", "Library artwork cache", filepath.Join("appcache", "librarycache"))
	for _, d := range []string{"logs", "dumps"} {
		entries, _ := os.ReadDir(filepath.Join(root, d))
		var files []string
		for _, e := range entries {
			files = append(files, filepath.Join(root, d, e.Name()))
		}
		add(Item{Key: d, Name: "Steam " + d, Kind: KindCache, Safe: true, Paths: files})
	}

	for _, d := range dirs(filepath.Join(root, "compatibilitytools.d")) {
		add(Item{Key: "proton:" + d, Name: d, Kind: KindProton, Paths: []string{filepath.Join(root, "compatibilitytools.d", d)}})
	}

	res.Items = append(res.Items, recordings(root, names)...)
	sort.SliceStable(res.Items, func(i, j int) bool { return res.Items[i].Size > res.Items[j].Size })
	return res
}

func recordingAppIDs(root string) []string {
	var ids []string
	bases, _ := filepath.Glob(filepath.Join(root, "userdata", "*", "gamerecordings", "video"))
	for _, base := range bases {
		for _, d := range dirs(base) {
			if m := recDir.FindStringSubmatch(d); m != nil {
				ids = append(ids, m[1])
			}
		}
	}
	return ids
}

var recDir = regexp.MustCompile(`^bg_(\d+)_(\d{8})_(\d{6})$`)

func recordings(root string, names map[string]string) []Item {
	var out []Item
	users, _ := filepath.Glob(filepath.Join(root, "userdata", "*", "gamerecordings"))
	for _, base := range users {
		for _, d := range dirs(filepath.Join(base, "video")) {
			p := filepath.Join(base, "video", d)
			it := Item{Key: "rec:" + p, Name: d, Kind: KindRecording, Paths: []string{p}}
			if m := recDir.FindStringSubmatch(d); m != nil {
				if t, err := time.ParseInLocation("20060102 150405", m[2]+" "+m[3], time.Local); err == nil {
					it.Name = fmt.Sprintf("%s recording, %s", appName(names, m[1]), t.Format("2006-01-02 15:04"))
				}
				// Steam names the matching timeline timeline_<appid><date>_<time>.json
				if tl := filepath.Join(base, "timelines", "timeline_"+m[1]+m[2]+"_"+m[3]+".json"); exists(tl) {
					it.Paths = append(it.Paths, tl)
				}
			}
			it.Size = sum(it.Paths)
			if it.Size > 0 {
				out = append(out, it)
			}
		}
	}
	return out
}

// Remove deletes everything in the item. Every path must sit inside root or one of the libraries,
// and may not be one of them, so a bad scan result can never delete outside Steam's folders.
func Remove(it Item, root string, libs []string, dryRun bool) error {
	allowed := append([]string{root}, libs...)
	for _, p := range it.Paths {
		ok := false
		for _, a := range allowed {
			rel, err := filepath.Rel(a, p)
			if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("refusing to delete outside Steam: %s", p)
		}
	}
	if dryRun {
		return nil
	}
	for _, p := range it.Paths {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
}

// Select returns the items whose kind is in kinds, never including anything outside that list.
func Select(items []Item, kinds []Kind) []Item {
	want := map[Kind]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	var out []Item
	for _, it := range items {
		if want[it.Kind] {
			out = append(out, it)
		}
	}
	return out
}

// ParseKinds turns "leftover,prefix" into kinds, rejecting names that do not exist.
func ParseKinds(csv string) ([]Kind, error) {
	valid := map[string]Kind{}
	for _, k := range []Kind{KindRecording, KindLeftover, KindPrefix, KindCache, KindProton, KindDownload} {
		valid[string(k)] = k
	}
	var out []Kind
	for _, f := range strings.Split(csv, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		k, ok := valid[f]
		if !ok {
			return nil, fmt.Errorf("unknown type %q (have recording, leftover, prefix, cache, proton, partial)", f)
		}
		out = append(out, k)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no types given")
	}
	return out, nil
}
