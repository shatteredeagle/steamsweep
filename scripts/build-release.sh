#!/usr/bin/env bash
# Usage: scripts/build-release.sh 0.1.0 [outdir]
# Cross-compiles steamsweep for linux/macOS/windows (amd64+arm64) into archives plus checksums.txt.
set -euo pipefail
version="${1:?usage: build-release.sh <version> [outdir]}"
version="${version#v}"
out="${2:-dist}"
rm -rf "$out" && mkdir -p "$out" && out="$(cd "$out" && pwd)"
stage="$(mktemp -d)"; trap 'rm -rf "$stage"' EXIT

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  os="${target%/*}"; arch="${target#*/}"
  bin=steamsweep; [[ $os == windows ]] && bin=steamsweep.exe
  d="$stage/$os-$arch"; mkdir -p "$d"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$d/$bin" .
  cp LICENSE README.md "$d/"
  if [[ $os == windows ]]; then
    (cd "$d" && zip -q "$out/steamsweep_${os}_${arch}.zip" "$bin" LICENSE README.md)
  else
    tar -C "$d" -czf "$out/steamsweep_${os}_${arch}.tar.gz" "$bin" LICENSE README.md
  fi
done
(cd "$out" && sha256sum steamsweep_* > checksums.txt)
ls -l "$out"
