#!/usr/bin/env bash
# Usage: scripts/release-notes.sh 0.5.0 [CHANGELOG.md]
# Prints the body of the "## 0.5.0 ..." section (up to the next "## "), trimmed.
# Exits 1 if the section is missing or empty: every release needs user-facing notes.
set -euo pipefail
version="${1:?usage: release-notes.sh <version> [changelog]}"
file="${2:-CHANGELOG.md}"
version="${version#v}"
[[ -f "$file" ]] || { echo "release-notes: $file not found" >&2; exit 1; }
notes="$(awk -v v="$version" '
  /^## / {
    if (found) exit
    h = substr($0, 4)
    if (h == v || index(h, v " ") == 1) { found = 1; next }
  }
  found { print }
' "$file")"
# trim leading/trailing blank lines
notes="$(printf '%s\n' "$notes" | sed -e '/./,$!d' | sed -e ':a' -e '/^[[:space:]]*$/{$d;N;ba' -e '}')"
if [[ -z "${notes//[[:space:]]/}" ]]; then
  echo "release-notes: no (or empty) '## $version' section in $file; write release notes first" >&2
  exit 1
fi
printf '%s\n' "$notes"
