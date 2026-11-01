# Changelog

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
