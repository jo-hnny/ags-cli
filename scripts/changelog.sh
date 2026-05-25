#!/usr/bin/env bash

set -euo pipefail

CHANGELOG_FILE="${CHANGELOG_FILE:-CHANGELOG.md}"
HEADING_RE='^## \[([0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z]+(\.[0-9]+)?)?)\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$'

usage() {
  cat <<'EOF'
Usage:
  scripts/changelog.sh validate [version]
  scripts/changelog.sh validate-release-notes <version>
  scripts/changelog.sh validate-latest-release-notes
  scripts/changelog.sh extract <version> <output>

Commands:
  validate          Validate changelog heading format and duplicate versions.
                    If version is provided, also require that section to exist.
  validate-release-notes
                    Validate the target version section uses the release-note
                    structure: Breaking Changes, Features, Bug Fixes, Docs.
  validate-latest-release-notes
                    Validate the latest changelog section uses the release-note
                    structure: Breaking Changes, Features, Bug Fixes, Docs.
  extract           Write the requested version section body to the output file.

Notes:
  - Versions use X.Y.Z format in CHANGELOG.md.
  - Tags use vX.Y.Z; callers should strip the leading "v" before invoking.
EOF
}

ensure_changelog_exists() {
  if [ ! -f "$CHANGELOG_FILE" ]; then
    echo "Missing changelog file: $CHANGELOG_FILE" >&2
    exit 1
  fi
}

latest_version() {
  ensure_changelog_exists

  awk '
    /^## \[/ {
      line = $0
      sub(/^## \[/, "", line)
      sub(/\] - .*/, "", line)
      print line
      exit
    }
  ' "$CHANGELOG_FILE"
}

validate() {
  local version="${1:-}"
  local headings versions duplicates

  ensure_changelog_exists

  headings="$(grep -n '^## \[' "$CHANGELOG_FILE" || true)"
  if [ -z "$headings" ]; then
    echo "No version headings found in $CHANGELOG_FILE" >&2
    exit 1
  fi

  if ! awk '
    /^## \[/ {
      if ($0 !~ /^## \[[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z]+(\.[0-9]+)?)?\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$/) {
        printf("Invalid changelog heading: %s\n", $0) > "/dev/stderr"
        bad = 1
      }
    }
    END { exit bad }
  ' "$CHANGELOG_FILE"; then
    exit 1
  fi

  versions="$(printf '%s\n' "$headings" | sed -E 's/^[0-9]+:## \[([^]]+)\].*/\1/')"
  duplicates="$(printf '%s\n' "$versions" | sort | uniq -d || true)"
  if [ -n "$duplicates" ]; then
    echo "Duplicate changelog versions found:" >&2
    printf '%s\n' "$duplicates" >&2
    exit 1
  fi

  if [ -n "$version" ] && ! grep -Eq "^## \[$version\] - " "$CHANGELOG_FILE"; then
    echo "Missing changelog section for version $version in $CHANGELOG_FILE" >&2
    exit 1
  fi
}

validate_release_notes() {
  local version sections

  if [ "$#" -ne 1 ]; then
    usage >&2
    exit 1
  fi

  version="$1"
  validate "$version"

  sections="$(awk -v ver="$version" '
    $0 ~ ("^## \\[" ver "\\] - ") { in_section = 1; next }
    /^## \[/ && in_section { exit }
    in_section && /^### / {
      sub(/^### /, "", $0)
      print
    }
  ' "$CHANGELOG_FILE")"

  if [ "$sections" != $'Breaking Changes\nFeatures\nBug Fixes\nDocs' ]; then
    echo "Release notes for version $version must use exactly these sections in order:" >&2
    echo "  ### Breaking Changes" >&2
    echo "  ### Features" >&2
    echo "  ### Bug Fixes" >&2
    echo "  ### Docs" >&2
    echo "Found:" >&2
    if [ -n "$sections" ]; then
      printf '  %s\n' "$sections" >&2
    else
      echo "  <none>" >&2
    fi
    exit 1
  fi
}

extract() {
  local version output

  if [ "$#" -ne 2 ]; then
    usage >&2
    exit 1
  fi

  version="$1"
  output="$2"

  validate "$version"

  awk -v ver="$version" '
    $0 ~ ("^## \\[" ver "\\] - ") { in_section = 1; next }
    /^## \[/ && in_section { exit }
    in_section { print }
  ' "$CHANGELOG_FILE" > "$output"

  if ! grep -q '[^[:space:]]' "$output"; then
    echo "Changelog section for version $version is empty" >&2
    exit 1
  fi
}

main() {
  if [ "$#" -lt 1 ]; then
    usage >&2
    exit 1
  fi

  case "$1" in
    validate)
      shift
      if [ "$#" -gt 1 ]; then
        usage >&2
        exit 1
      fi
      validate "${1:-}"
      ;;
    validate-release-notes)
      shift
      validate_release_notes "$@"
      ;;
    validate-latest-release-notes)
      shift
      if [ "$#" -ne 0 ]; then
        usage >&2
        exit 1
      fi
      validate_release_notes "$(latest_version)"
      ;;
    extract)
      shift
      extract "$@"
      ;;
    *)
      usage >&2
      exit 1
      ;;
  esac
}

main "$@"
