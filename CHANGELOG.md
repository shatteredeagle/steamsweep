# Changelog

## 0.4.0

- `steamsweep clean --older-than 30d` (or `2w`, `12h`) limits recordings to those older than that, so a timer can delete old recordings with `--include recording` and keep recent ones. It only applies to recordings.
- The scanner now also looks in Steam's saved-clips folder, not only the background recording folder.

## 0.3.0

- New `steamsweep clean` command for unattended cleanup, for example on a timer. By default it deletes only leftovers of uninstalled games (orphaned game folders, shader cache and Workshop data); `--include` opts in to other types such as Proton prefixes. Supports `--dry-run`, `--yes`, `--close-steam` and `--if-closed`. The README has a ready-made systemd timer.

## 0.2.1

- Leftovers, Proton prefixes and recordings of uninstalled games are now named (for example "Valheim Proton prefix") using Steam's local app database, instead of showing a bare app id.
- Cleanup tab: `m` marks every item of the type under the cursor, so you can clear all Proton prefixes of uninstalled games in one go. The confirm screen warns that prefixes can hold save files.
- Stopped listing Steam's placeholder `compatdata/0` as a leftover.

## 0.2.0

- New Cleanup tab (press `tab`): recordings, leftovers from uninstalled games, Proton prefixes, caches, logs, incomplete downloads and old Proton builds, with the same mark-and-confirm flow. Marks carry across tabs, so you can clear games and clutter in one pass.
- `a` on the Cleanup tab marks only the regenerable and ownerless items; recordings, prefixes, partial downloads and Proton builds are always marked by hand.
- Leftover detection turns itself off with a warning when a configured library is missing (for example an unmounted drive).
- Built-in updates: checks GitHub for a newer release at most once a day and shows a banner; `u` or `steamsweep update` installs it after verifying the checksum. `steamsweep update --auto on` installs new releases automatically; `--no-update-check` turns the check off.
- `--list-cleanup` prints the cleanup candidates without opening the UI.

## 0.1.0

First release.

- Terminal UI that lists every installed Steam game across all your libraries, biggest first, with a size bar.
- Move with the arrow keys, mark games with space, press enter to uninstall them all at once.
- Filter by name with `/`, cycle the sort with `s` (size, last played, last updated, name) and reverse it with `r`; each row shows when you last played it, show Proton and runtime tools with `t`.
- Optionally delete the Proton prefix (it can hold saves); it is kept by default.
- Close Steam from the confirm screen with `c`, or automatically with `--close-steam`.
- `--dry-run` runs the whole UI without deleting anything; `--list` prints installed games.
- Builds for Linux, macOS and Windows.
- One-line installers for Linux/macOS (`install.sh`) and Windows (`install.ps1`) that verify checksums, or `go install`.
