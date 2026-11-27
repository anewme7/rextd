#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  printf 'Usage: sh scripts/package.sh OUTPUT_DIRECTORY\n' >&2
  exit 2
fi

mkdir -p "$1"
output=$(cd "$1" && pwd)
cd "$(dirname "$0")/.."
platform=$(go env GOOS)
arch=$(go env GOARCH)
if [ "$platform" = darwin ] && [ "$(go env CGO_ENABLED)" != 1 ]; then
  printf 'Build macOS packages with CGO enabled for native CPU readings.\n' >&2
  exit 1
fi

name="rex-td_${platform}_${arch}"
version=${REX_TD_VERSION:-dev}
case "$version" in
  *[!a-zA-Z0-9.+-]*)
    printf 'Invalid REX_TD_VERSION.\n' >&2
    exit 2
    ;;
esac
stage=$(mktemp -d "$output/.rex-td-package.XXXXXX")
trap 'rm -r "$stage"' EXIT
mkdir -p "$stage/$name/rex"
go build -trimpath -ldflags "-X main.version=$version" -o "$stage/$name/rextd" ./cmd/rex-td-preview
cp README.md README.zh-CN.md "$stage/$name/"
mkdir -p "$stage/$name/docs"
cp docs/user-guide.md "$stage/$name/docs/"
cp -R docs/screenshots "$stage/$name/docs/"
cp rex/action.lua rex/init.example.lua rex/metrics.lua rex/claude-statusline.example.json "$stage/$name/rex/"
mkdir -p "$stage/$name/rex/agent-details"
cp rex/palette.lua rex/current-task.lua scripts/launch.sh "$stage/$name/rex/"
cp rex/agent-details/action.lua rex/agent-details/read.mjs "$stage/$name/rex/agent-details/"
go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}{{"\t"}}{{.Dir}}{{end}}{{end}}' ./cmd/rex-td-preview > "$stage/modules"
sort -u "$stage/modules" | while IFS="$(printf '\t')" read -r module source; do
  [ -n "$module" ] || continue
  for file in "$source"/LICENSE* "$source"/COPYING* "$source"/NOTICE* "$source"/PATENTS; do
    [ -f "$file" ] || continue
    mkdir -p "$stage/$name/licenses/$module"
    cp "$file" "$stage/$name/licenses/$module/"
  done
done
mkdir -p "$stage/$name/licenses/go"
goroot=$(go env GOROOT)
go_license="$goroot/LICENSE"
if [ ! -f "$go_license" ]; then
  go_license="$goroot/../LICENSE"
fi
cp "$go_license" "$stage/$name/licenses/go/"
if [ -f "$goroot/PATENTS" ]; then
  cp "$goroot/PATENTS" "$stage/$name/licenses/go/"
fi
if [ -f LICENSE ]; then
  cp LICENSE "$stage/$name/"
fi
# Store neutral ownership and skip filesystem attributes on both tar variants.
case $(tar --version) in
  *bsdtar*) set -- --uid 0 --gid 0 --uname root --gname root ;;
  *) set -- --owner=0 --group=0 --numeric-owner ;;
esac
COPYFILE_DISABLE=1 tar "$@" --no-xattrs -czf "$stage/$name.tar.gz" -C "$stage" "$name"
(
  cd "$stage"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$name.tar.gz"
  else
    shasum -a 256 "$name.tar.gz"
  fi
) > "$stage/$name.tar.gz.sha256"
mv "$stage/$name.tar.gz" "$stage/$name.tar.gz.sha256" "$output"
printf '%s\n' "$output/$name.tar.gz"
