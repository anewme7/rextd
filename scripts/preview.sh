#!/bin/sh
set -eu
umask 077
usage() {
  echo 'Usage: ./scripts/preview.sh [--demo | --copy-prod-data]' >&2
}
case "$#:${1:-}" in
  0:|1:--demo|1:--copy-prod-data) ;;
  1:--help) usage; exit 0 ;;
  *) usage; exit 2 ;;
esac

work="$HOME/dev/rextd/work"
preview="$work/preview"
export TMPDIR="$work/temp/go"
# Refuse redirected workspace directories before creating anything below them.
for dir in "$work" "$preview" "$work/temp" "$TMPDIR"; do
  [ ! -L "$dir" ] || { echo "Unexpected workspace symlink: $dir" >&2; exit 1; }
  mkdir -p "$dir"
done
cd "$(dirname "$0")/.."
export REX_TD_DATA_DIR="$preview/data"
export REX_TD_LANG="${REX_TD_LANG:-zh}"
if [ -L "$preview/rextd" ] || [ -d "$preview/rextd" ]; then
  echo 'Invalid preview binary destination.' >&2
  exit 1
fi
go build -o "$preview/rextd" ./cmd/rex-td-preview

case "${1:-}" in
  --copy-prod-data)
    run=$(mktemp -d "$preview/copy.XXXXXX")
    trap 'if [ -d "$run/snapshot" ]; then chmod -R u+w "$run/snapshot"; fi; rm -rf "$run"' EXIT
    trap 'exit 129' HUP
    trap 'exit 130' INT
    trap 'exit 143' TERM
    python3 scripts/preview_snapshot.py "$HOME/dev/rextd/production/data" "$run"
    export REX_TD_DATA_DIR="$run/data"
    "$preview/rextd" --home
    ;;
  *)
    if [ -L "$REX_TD_DATA_DIR" ] || { [ -d "$REX_TD_DATA_DIR" ] &&
        [ -n "$(find "$REX_TD_DATA_DIR" \( -type l -o \( -type f -links +1 \) \) -print -quit)" ]; }; then
      echo 'Preview data contains a link to another location.' >&2
      exit 1
    fi
    mkdir -p "$REX_TD_DATA_DIR"
    exec "$preview/rextd" --home "$@"
    ;;
esac
