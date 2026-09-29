#!/bin/sh
# Builds a Go main package for the browser.
#
#   web/build.sh [-o out-dir] [-a asset-dir]... <package>
#
# e.g. web/build.sh ./examples/cube writes build/web/cube/. Serve that
# directory over HTTP (python3 -m http.server -d build/web/cube 8080). Needs
# emscripten (emcc) on PATH.
#
# Each -a directory (relative to the repository root, like the paths the game
# loads) is bundled into the page's filesystem at that same path, e.g.
# web/build.sh -a examples/assets ./examples/bee.
#
# The page runs Go's wasm plus one emscripten module per C library: raylib
# always, Jolt when the package uses physics (see lib.sh).
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
web=$root/web
out=
assets= # asset directories, one per line
nl='
'
while [ $# -gt 1 ]; do
	case $1 in
	-o) out=$2 ;;
	-a) assets="$assets${2%/}$nl" ;;
	*) break ;;
	esac
	shift 2
done
if [ $# -ne 1 ]; then
	echo "usage: web/build.sh [-o out-dir] [-a asset-dir]... <package>" >&2
	exit 2
fi
pkg=$1
: "${out:=$root/build/web/$(basename "$pkg")}"
mkdir -p "$out"

. "$web/lib.sh"

# Turn the asset directories into --preload-file arguments, kept whole even
# if the paths contain spaces.
set --
while IFS= read -r dir; do
	if [ -n "$dir" ]; then
		set -- "$@" --preload-file "$root/$dir@/$dir"
	fi
done <<ASSETS
$assets
ASSETS

modules=raylib
rm -f "$out/raylib.data"
build_raylib "$out" web "$@"
rm -f "$out/jolt.js" "$out/jolt.wasm"
if uses_jolt "$pkg"; then
	build_jolt "$out" web
	modules="$modules jolt"
fi

echo "go build $pkg"
(cd "$root" && GOOS=js GOARCH=wasm go build -o "$out/game.wasm" "$pkg")
cp -f "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$web/fs.js" "$out/"
list=$(printf '"%s",' $modules)
sed -e "s|__TITLE__|Illusion: $(basename "$pkg")|" -e "s|__MODULES__|[${list%,}]|" \
	"$web/index.html" >"$out/index.html"
echo "built $out"
