# steamsweep

A terminal UI to bulk-uninstall Steam games. Arrow keys to move, space to mark, enter to uninstall.

Linux first; also builds for macOS and Windows (path detection included, less tested).

## Install

```sh
go install steamsweep@latest   # needs Go 1.25+; replace with the module path once published
```

or grab a binary from the releases page (tag `vX.Y.Z` and CI builds Linux, macOS and Windows archives; `scripts/build-release.sh X.Y.Z` does the same locally), or build locally:

```sh
go build -o steamsweep . && install -m755 steamsweep ~/.local/bin/
```

## Use

```
steamsweep              # interactive
steamsweep --dry-run    # full UI, deletes nothing
steamsweep --close-steam  # close Steam automatically before uninstalling
steamsweep --list       # print installed games
steamsweep --steam-root /path/to/Steam
```

| Key | Action |
| --- | --- |
| `↑` `↓` / `j` `k` | move (`pgup` `pgdn` `g` `G` also work) |
| `space` | mark / unmark, then move down |
| `a` / `i` / `c` | mark all shown / invert / clear |
| `/` | filter by name (`esc` clears) |
| `s` / `r` | cycle sort: size, last played, last updated, name / reverse it |
| `t` | show Proton and runtime tools (hidden by default) |
| `enter` | review and uninstall |

Uninstall deletes `steamapps/common/<installdir>` and the app's `appmanifest`. The Proton prefix
(`compatdata`, may contain saves) is kept unless you turn on the prefix option in the confirm screen.
Steam rewrites its library files while running, so close it first. On the confirm screen press `c` to close it (polite shutdown, then terminate, then kill), or start with `--close-steam` to do it automatically.

Reads every library in `libraryfolders.vdf`. Looks in the standard Linux (incl. Flatpak, Snap), macOS and Windows locations.

## License

GPL-3.0-or-later. See `LICENSE`.
