package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"steamsweep/internal/steam"
	"steamsweep/internal/ui"
)

// version is set by release builds via -ldflags "-X main.version=...".
var version = ""

func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

func main() {
	root := flag.String("steam-root", "", "Steam install directory (default: auto-detect)")
	dry := flag.Bool("dry-run", false, "go through the motions without deleting anything")
	closeSteam := flag.Bool("close-steam", false, "close Steam automatically before uninstalling")
	list := flag.Bool("list", false, "print installed games and exit")
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
	games := steam.Installed(steam.Libraries(r))
	steam.ApplyLastPlayed(r, games)
	if *list {
		for _, g := range games {
			fmt.Printf("%-10s %12d %11d  %s\n", g.AppID, g.Size, g.LastPlayed, g.Name)
		}
		return
	}
	if len(games) == 0 {
		fmt.Fprintln(os.Stderr, "steamsweep: no installed games found in", r)
		os.Exit(1)
	}
	if err := ui.Run(games, ui.Options{DryRun: *dry, CloseSteam: *closeSteam, SteamRunning: steam.Running(), Version: buildVersion()}); err != nil {
		fmt.Fprintln(os.Stderr, "steamsweep:", err)
		os.Exit(1)
	}
}
