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
steamsweep --list          # print installed games and exit
steamsweep --steam-root /path/to/Steam
```

### Keys

| Key | Action |
| --- | --- |
| `↑` `↓` or `j` `k` | Move (`pgup` `pgdn` `g` `G` also work) |
| `space` | Mark or unmark, then move down |
| `a` / `i` / `c` | Mark everything shown / invert / clear marks |
| `/` | Filter by name (`esc` clears) |
| `s` / `r` | Cycle sort: size, last played, last updated, name / reverse it |
| `t` | Show Proton and runtime tools (hidden by default) |
| `enter` | Review and confirm the uninstall |

On the confirm screen: `y` uninstalls, `p` also deletes the Proton prefix, `c` closes Steam, `esc` cancels.

## What it deletes

For each marked game:

1. `steamapps/common/<installdir>`, the game files.
2. `steamapps/appmanifest_<appid>.acf`, last, so a failed delete leaves the game visible to Steam instead of half gone.
3. Only if you turn the option on: `compatdata/<appid>` (the Proton prefix, which can hold saves) and `shadercache/<appid>`.

Deletion is permanent, not a move to the trash; the confirm screen says so and shows the space you will free.
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
