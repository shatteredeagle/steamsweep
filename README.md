```text
         __                                                   
   _____/ /____  ____ _____ ___  ______      _____  ___  ____ 
  / ___/ __/ _ \/ __ `/ __ `__ \/ ___/ | /| / / _ \/ _ \/ __ \
 (__  ) /_/  __/ /_/ / / / / / (__  )| |/ |/ /  __/  __/ /_/ /
/____/\__/\___/\__,_/_/ /_/ /_/____/ |__/|__/\___/\___/ .___/ 
                                                     /_/      
```

**Bulk-uninstall Steam games from the terminal.** Arrow keys to move, space to mark, enter to sweep them all away.

[![ci](https://github.com/ShatteredEagle/steamsweep/actions/workflows/ci.yml/badge.svg)](https://github.com/ShatteredEagle/steamsweep/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/ShatteredEagle/steamsweep)](https://github.com/ShatteredEagle/steamsweep/releases)
[![license](https://img.shields.io/badge/license-GPL--3.0-blue)](LICENSE)

```text
 steamsweep  122 games · sort: last played ↑

   ▌ ● Control Ultimate Edition     ━━━━━━━━━━━━   45.0 GB     never
     ● STAR WARS: Galactic Racer    ━━━━━━━━━━━─   41.7 GB    4mo ago
     ○ No Man's Sky                 ━━━━━━━━────   33.6 GB    2mo ago
     ○ Last Epoch                   ━━━━━───────   24.2 GB    3d ago

 2 selected · frees 86.7 GB
 ↑↓ move · space mark · a all · i invert · / filter · s sort · r reverse · enter uninstall · q quit
```

Steam's own library can only uninstall one game at a time, or makes you hunt through a list. steamsweep shows
everything you have installed with sizes and when you last played it, lets you mark as many as you like, and
removes them in one go.

## Install

**Linux / macOS**

```sh
curl -fsSL https://raw.githubusercontent.com/ShatteredEagle/steamsweep/master/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/ShatteredEagle/steamsweep/master/install.ps1 | iex
```

**With Go 1.26+**

```sh
go install github.com/ShatteredEagle/steamsweep@latest
```

Or download an archive from the [releases page](https://github.com/ShatteredEagle/steamsweep/releases)
(Linux, macOS and Windows, amd64 and arm64, with checksums). The install scripts verify the SHA-256 checksum
before installing, and accept `VERSION=v0.1.0` to pin a release.

## Use

```sh
steamsweep                 # interactive
steamsweep --dry-run       # run the whole UI, delete nothing (try this first)
steamsweep --close-steam   # close Steam automatically before uninstalling
steamsweep clean          # no UI: delete leftovers of uninstalled games (see Automatic cleanup)
steamsweep update          # install the latest release
steamsweep --list          # print installed games and exit
steamsweep --list-cleanup  # print cleanup candidates and exit (read-only)
steamsweep --steam-root /path/to/Steam
```

### Keys

| Key | Action |
| --- | --- |
| `↑` `↓` or `j` `k` | Move (`pgup` `pgdn` `g` `G` also work) |
| `space` | Mark or unmark, then move down |
| `a` / `i` / `c` | Mark everything shown / invert / clear marks |
| `/` | Filter by name (`esc` clears) |
| `s` / `r` | Cycle sort / reverse it (Games: size, last played, last updated, name. Cleanup: size, type, name) |
| `m` | Cleanup tab: mark every item of the row's type, such as all Proton prefixes (again to unmark) |
| `tab` | Switch between the Games and Cleanup tabs |
| `t` | Show Proton and runtime tools on the Games tab (hidden by default) |
| `enter` | Review and confirm the uninstall |

On the confirm screen: `y` uninstalls, `p` also deletes the Proton prefix, `c` closes Steam, `esc` cancels.

## Automatic cleanup

`steamsweep clean` does the Cleanup tab's job with no UI, for scripts and timers. By default it deletes only
`leftover` items: game folders with no manifest, plus shader cache and Workshop data of uninstalled games. Nothing that might hold saves is touched unless you ask.

```sh
steamsweep clean --dry-run                      # show what it would delete
steamsweep clean                                # same list, then asks y/N
steamsweep clean --yes --if-closed              # unattended: skip quietly if Steam is running
steamsweep clean --yes --close-steam            # close Steam first, then clean
steamsweep clean --include leftover,cache,prefix --yes   # opt in to more types
steamsweep clean --include leftover,recording --older-than 30d --yes   # also delete recordings older than 30 days
```

The `recording` type is Steam's background recordings and saved clips (`userdata/*/gamerecordings`) with their timeline files. They are your own videos, so unattended runs should pair it with `--older-than` (`30d`, `2w`, `12h`), which keeps anything newer and only applies to recordings.

`--include` takes `leftover`, `cache`, `prefix`, `recording`, `proton` and `partial`. Add `prefix` to also remove the Proton
prefixes of uninstalled games, which can contain save files. Exit codes: 0 done or nothing to do, 1 an error (including Steam running without
`--close-steam` or `--if-closed`), 2 bad arguments.

To run it weekly on Linux, create two files in `~/.config/systemd/user/`:

```ini
# steamsweep-clean.service
[Unit]
Description=Clean leftovers of uninstalled Steam games

[Service]
Type=oneshot
ExecStart=%h/.local/bin/steamsweep clean --yes --if-closed
```

```ini
# steamsweep-clean.timer
[Unit]
Description=Weekly steamsweep cleanup

[Timer]
OnCalendar=weekly
Persistent=true

[Install]
WantedBy=timers.target
```

```sh
systemctl --user daemon-reload && systemctl --user enable --now steamsweep-clean.timer
```

On Windows, `schtasks /Create /SC WEEKLY /TN steamsweep-clean /TR "steamsweep clean --yes --if-closed"` does the same; on macOS use `cron` or `launchd`.

## Updates

steamsweep checks GitHub for a newer release when it starts, at most once a day, and shows `⬆ vX.Y.Z available` in
the header. Press `u` to install it, or run `steamsweep update` (`--check` only reports). The download is verified
against the release's SHA-256 checksum before the binary is replaced.

```sh
steamsweep update --auto on    # install new releases automatically at startup (off to disable)
steamsweep --no-update-check   # skip the check (or set STEAMSWEEP_NO_UPDATE_CHECK=1)
```

The check is one request to the GitHub API and sends nothing about you. Builds made with `go build` (version `dev`)
never check. If steamsweep is installed somewhere you can't write to, such as a system package, update it there instead.

## Cleanup tab

Press `tab` for everything else that eats disk space. Items are scanned when you first open the tab and use the
same mark-and-confirm flow, so you can clear games and clutter in one pass. Marks carry across tabs.

| Type | What it is | `a` marks it? |
| --- | --- | --- |
| `recording` | Game recordings and their timeline files (`userdata/*/gamerecordings/video`) | no |
| `leftover` | Game folders with no manifest, plus shader cache and Workshop data for uninstalled games | yes |
| `prefix` | Proton prefixes of uninstalled games (can hold saves); press `m` on one to mark them all | no |
| `cache` | Web, store and artwork caches, logs, crash dumps, shader caches of installed games | yes |
| `partial` | Incomplete downloads (`steamapps/downloading`, `temp`) | no |
| `proton` | Custom Proton builds in `compatibilitytools.d` (a game may still use one) | no |

`a` only marks the types that are safe to regenerate or have no owner; recordings, prefixes, partial downloads and
Proton builds are always marked by hand (they show in yellow). Non-Steam shortcut prefixes are never listed.
If a library listed in `libraryfolders.vdf` is missing, for example an unmounted drive, leftover detection turns
itself off with a warning, because that drive's games would otherwise look uninstalled.

Uninstalled games are named from Steam's local app database (`appcache/appinfo.vdf`), so you see "Valheim Proton prefix" instead of an app id. Apps Steam no longer knows show as `App <id>`.

Every path is checked to sit inside Steam's folders before anything is deleted.

## What it deletes

For each marked game:

1. `steamapps/common/<installdir>`, the game files.
2. `steamapps/appmanifest_<appid>.acf`, last, so a failed delete leaves the game visible to Steam instead of half gone.
3. Only if you turn the option on: `compatdata/<appid>` (the Proton prefix, which can hold saves) and `shadercache/<appid>`.

Games are removed as described above. Deletion is permanent, not a move to the trash; the confirm screen says so and shows the space you will free.
Steam rewrites its library files while it is running, so close it first: press `c` on the confirm screen or
start with `--close-steam` (polite shutdown, then terminate, then kill). If Steam will not close, nothing is deleted.

## How it works

- Finds Steam in the usual places on Linux (including Flatpak and Snap), macOS and Windows, or use `--steam-root` / `$STEAM_ROOT`.
- Reads every library listed in `libraryfolders.vdf`, so games on other drives show up.
- Reads each `appmanifest_*.acf` for name, size and install folder.
- Takes last-played from the newer of the manifest and each user's `localconfig.vdf`.
- Hides Proton, Steam Linux Runtime and Steamworks redistributables unless you press `t`.

Linux is the primary target and the one I use. macOS and Windows builds exist and use the standard Steam paths, but get less testing.

## Build

```sh
git clone https://github.com/ShatteredEagle/steamsweep && cd steamsweep
go build -o steamsweep .
go test ./...
scripts/build-release.sh 0.1.0   # cross-compiled archives in dist/
```

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Releasing

Add a `## X.Y.Z` section to `CHANGELOG.md`, then `git tag vX.Y.Z && git push --tags`. CI builds the archives and
publishes the release with those notes.

## License

[GPL-3.0](LICENSE)
