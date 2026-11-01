#!/bin/sh
# steamsweep installer for Linux and macOS.
#   curl -fsSL https://raw.githubusercontent.com/ShatteredEagle/steamsweep/master/install.sh | sh
# Env: VERSION=v0.1.0 to pin a release (default: latest), BIN_DIR to change the install dir.
set -eu

REPO="ShatteredEagle/steamsweep"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in linux|darwin) ;; *) echo "unsupported OS: $os (Windows: see install.ps1)" >&2; exit 1 ;; esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

if [ -n "${VERSION:-}" ]; then
  base="https://github.com/$REPO/releases/download/$VERSION"
else
  base="https://github.com/$REPO/releases/latest/download"
fi
file="steamsweep_${os}_${arch}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "Downloading $file ..."
curl -fsSL "$base/$file" -o "$tmp/$file"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

# Verify the download against the published checksum.
want=$(grep " $file\$" "$tmp/checksums.txt" | cut -d' ' -f1)
if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum "$tmp/$file" | cut -d' ' -f1)
else got=$(shasum -a 256 "$tmp/$file" | cut -d' ' -f1); fi
[ -n "$want" ] && [ "$want" = "$got" ] || { echo "checksum mismatch, aborting" >&2; exit 1; }

tar -xzf "$tmp/$file" -C "$tmp" steamsweep
mkdir -p "$BIN_DIR"
install -m 755 "$tmp/steamsweep" "$BIN_DIR/steamsweep"
echo "Installed steamsweep to $BIN_DIR/steamsweep"
case ":$PATH:" in *":$BIN_DIR:"*) ;; *) echo "Note: $BIN_DIR is not on your PATH." ;; esac
