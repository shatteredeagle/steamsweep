#!/bin/sh
# Installs the latest steamsweep release to ~/.local/bin (override with PREFIX_BIN).
# Needs REPO_URL pointing at the release host; see README.
set -eu
BIN_DIR="${PREFIX_BIN:-$HOME/.local/bin}"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$(uname -m)" in x86_64|amd64) ARCH=amd64 ;; aarch64|arm64) ARCH=arm64 ;; *) echo "unsupported arch" >&2; exit 1 ;; esac
: "${REPO_URL:?set REPO_URL to the releases download base, e.g. https://github.com/OWNER/steamsweep/releases/latest/download}"
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$REPO_URL/steamsweep_${OS}_${ARCH}.tar.gz" | tar -xz -C "$tmp" steamsweep
mkdir -p "$BIN_DIR" && install -m 755 "$tmp/steamsweep" "$BIN_DIR/steamsweep"
echo "installed to $BIN_DIR/steamsweep"
