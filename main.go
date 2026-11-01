package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/ShatteredEagle/steamsweep/internal/clean"
	"github.com/ShatteredEagle/steamsweep/internal/steam"
	"github.com/ShatteredEagle/steamsweep/internal/ui"
	"github.com/ShatteredEagle/steamsweep/internal/update"
)

// version is set by release builds via -ldflags "-X main.version=...".
var version = ""

func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		v := strings.TrimPrefix(bi.Main.Version, "v")
		if strings.ContainsAny(v, "-+") {
			return "dev" // pseudo-version from an untagged or dirty checkout: never self-update those
		}
		return v
	}
	return "dev"
}

func runUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	check := fs.Bool("check", false, "only report whether a newer release exists")
	auto := fs.String("auto", "", "turn automatic updates on or off (on|off)")
	fs.Parse(args)

	if *auto != "" {
		cfg := update.LoadConfig()
		switch *auto {
		case "on":
			cfg.AutoUpdate = true
		case "off":
			cfg.AutoUpdate = false
		default:
			fmt.Fprintln(os.Stderr, `steamsweep: --auto takes "on" or "off"`)
			os.Exit(2)
		}
		if err := update.SaveConfig(cfg); err != nil {
			fmt.Fprintln(os.Stderr, "steamsweep:", err)
			os.Exit(1)
		}
		fmt.Println("automatic updates:", *auto)
		return
	}

	cur := buildVersion()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	u := update.Updater{}
	rel, err := u.Latest(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "steamsweep: could not check for updates:", err)
		os.Exit(1)
	}
	if !update.Newer(cur, rel.Tag) {
		fmt.Printf("steamsweep %s is up to date (latest release %s)\n", cur, rel.Tag)
		return
	}
	if *check {
		fmt.Printf("update available: %s -> %s\nrun `steamsweep update` to install it\n", cur, rel.Tag)
		return
	}
	fmt.Printf("updating %s -> %s ...\n", cur, rel.Tag)
	if err := u.Apply(ctx, rel); err != nil {
		fmt.Fprintln(os.Stderr, "steamsweep: update failed:", err)
		os.Exit(1)
	}
	fmt.Println("updated to", rel.Tag)
}

// runClean is the unattended mode: delete cleanup items of the chosen types, no UI.
// By default only "leftover" (folders and caches of uninstalled games) is removed, so it is safe on a timer.
func runClean(args []string) {
	fs := flag.NewFlagSet("clean", flag.ExitOnError)
	root := fs.String("steam-root", "", "Steam install directory (default: auto-detect)")
	include := fs.String("include", "leftover", "comma-separated types to delete: leftover, cache, prefix, recording, proton, partial")
	olderThan := fs.String("older-than", "", "limit recordings to those older than this, e.g. 30d, 2w, 12h")
	dry := fs.Bool("dry-run", false, "list what would be deleted, delete nothing")
	yes := fs.Bool("yes", false, "do not ask for confirmation (needed for timers and scripts)")
	closeSteam := fs.Bool("close-steam", false, "close Steam first if it is running")
	ifClosed := fs.Bool("if-closed", false, "exit quietly with success when Steam is running instead of failing")
	fs.Parse(args)

	kinds, err := clean.ParseKinds(*include)
	if err != nil {
		fmt.Fprintln(os.Stderr, "steamsweep:", err)
		os.Exit(2)
	}
	var age time.Duration
	if *olderThan != "" {
		var aerr error
		if age, aerr = clean.ParseAge(*olderThan); aerr != nil {
			fmt.Fprintln(os.Stderr, "steamsweep:", aerr)
			os.Exit(2)
		}
	}
	r, err := steam.FindRoot(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "steamsweep:", err)
		os.Exit(1)
	}
	libs := steam.Libraries(r)
	res := clean.Scan(r, libs, steam.Installed(libs))
	if *olderThan != "" {
		res.Items = clean.OlderThan(res.Items, age, time.Now())
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	items := clean.Select(res.Items, kinds)
	var total int64
	for _, it := range items {
		fmt.Printf("%-9s %10s  %s\n", it.Kind, humanSize(it.Size), it.Name)
		total += it.Size
	}
	if len(items) == 0 {
		fmt.Println("nothing to clean")
		return
	}
	noun := "items"
	if len(items) == 1 {
		noun = "item"
	}
	fmt.Printf("%d %s, %s\n", len(items), noun, humanSize(total))
	if *dry {
		return
	}

	if steam.Running() {
		switch {
		case *closeSteam:
			fmt.Println("closing Steam ...")
			if err := steam.Close(); err != nil {
				fmt.Fprintln(os.Stderr, "steamsweep:", err)
				os.Exit(1)
			}
		case *ifClosed:
			fmt.Println("Steam is running; skipping")
			return
		default:
			fmt.Fprintln(os.Stderr, "steamsweep: Steam is running; close it, or use --close-steam or --if-closed")
			os.Exit(1)
		}
	}
	if !*yes {
		fmt.Print("Delete these permanently? [y/N] ")
		var ans string
		fmt.Scanln(&ans)
		if ans != "y" && ans != "Y" {
			fmt.Println("cancelled")
			return
		}
	}
	var freed int64
	failed := 0
	for _, it := range items {
		if err := clean.Remove(it, r, libs, false); err != nil {
			fmt.Fprintf(os.Stderr, "failed: %s: %v\n", it.Name, err)
			failed++
			continue
		}
		freed += it.Size
	}
	fmt.Printf("freed %s", humanSize(freed))
	if failed > 0 {
		fmt.Printf(", %d failed", failed)
	}
	fmt.Println()
	if failed > 0 {
		os.Exit(1)
	}
}

func humanSize(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "update":
			runUpdate(os.Args[2:])
			return
		case "clean":
			runClean(os.Args[2:])
			return
		}
	}
	root := flag.String("steam-root", "", "Steam install directory (default: auto-detect)")
	dry := flag.Bool("dry-run", false, "go through the motions without deleting anything")
	closeSteam := flag.Bool("close-steam", false, "close Steam automatically before uninstalling")
	list := flag.Bool("list", false, "print installed games and exit")
	listClean := flag.Bool("list-cleanup", false, "print cleanup candidates (recordings, caches, leftovers) and exit")
	noUpdate := flag.Bool("no-update-check", false, "do not check GitHub for a newer release")
	showVer := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVer {
		fmt.Println("steamsweep", buildVersion())
		return
	}
	r, err := steam.FindRoot(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "steamsweep:", err)
		os.Exit(1)
	}
	libs := steam.Libraries(r)
	games := steam.Installed(libs)
	steam.ApplyLastPlayed(r, games)
	if *list {
		for _, g := range games {
			fmt.Printf("%-10s %12d %11d  %s\n", g.AppID, g.Size, g.LastPlayed, g.Name)
		}
		return
	}
	if *listClean {
		res := clean.Scan(r, libs, games)
		for _, it := range res.Items {
			mark := " "
			if it.Safe {
				mark = "*" // included by "mark all"
			}
			fmt.Printf("%s %-9s %12d  %s\n", mark, it.Kind, it.Size, it.Name)
		}
		for _, w := range res.Warnings {
			fmt.Fprintln(os.Stderr, "warning:", w)
		}
		return
	}
	if len(games) == 0 {
		fmt.Fprintln(os.Stderr, "steamsweep: no installed games found in", r)
		os.Exit(1)
	}
	if err := ui.Run(games, ui.Options{CheckUpdates: !*noUpdate && os.Getenv("STEAMSWEEP_NO_UPDATE_CHECK") == "", Root: r, Libs: libs, DryRun: *dry, CloseSteam: *closeSteam, SteamRunning: steam.Running(), Version: buildVersion()}); err != nil {
		fmt.Fprintln(os.Stderr, "steamsweep:", err)
		os.Exit(1)
	}
}
