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

func main() {
	if len(os.Args) > 1 && os.Args[1] == "update" {
		runUpdate(os.Args[2:])
		return
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
