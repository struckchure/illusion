#!/bin/sh
# Builds a Go main package for the browser.
#
#   web/build.sh [-m module-dir] [-o out-dir] [-a asset-dir]... [-t title] <package>
#
# e.g. web/build.sh ./examples/cube writes build/web/cube/. Serve that
# directory over HTTP (python3 -m http.server -d build/web/cube 8080). Needs
# emscripten (emcc) on PATH.
#
# -m builds from another module that depends on illusion (a game in its own
# repository); it defaults to illusion itself. The package, the asset
# directories and the default output directory are relative to it.
#
# Each -a directory (relative to the module, like the paths the game loads)
# is bundled into the page's filesystem at that same path, e.g.
# web/build.sh -a examples/assets ./examples/bee.
#
# The page runs Go's wasm plus one emscripten module per C library: raylib
# always, Jolt when the package uses physics (see lib.sh).
set -eu

web=$(cd "$(dirname "$0")" && pwd)
mod=$(cd "$web/.." && pwd)
out=
assets= # asset directories, one per line
title=
nl='
'
while [ $# -gt 1 ]; do
	case $1 in
	-m) mod=$(cd "$2" && pwd) ;;
	-o) out=$2 ;;
	-a) assets="$assets${2%/}$nl" ;;
	-t) title=$2 ;;
	*) break ;;
	esac
	shift 2
done
if [ $# -ne 1 ]; then
	echo "usage: web/build.sh [-m module-dir] [-o out-dir] [-a asset-dir]... [-t title] <package>" >&2
	exit 2
fi
pkg=$1
name=$(basename "$(cd "$mod" && cd "$pkg" && pwd)")
: "${out:=$mod/build/web/$name}"
: "${title:=Illusion: $name}"
mkdir -p "$out"
out=$(cd "$out" && pwd)

. "$web/lib.sh"

# Turn the asset directories into --preload-file arguments, kept whole even
# if the paths contain spaces.
set --
while IFS= read -r a; do
	if [ -n "$a" ]; then
		set -- "$@" --preload-file "$mod/$a@/$a"
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
(cd "$mod" && GOOS=js GOARCH=wasm go build -o "$out/game.wasm" "$pkg")
cp -f "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$web/fs.js" "$out/"
chmod u+w "$out/wasm_exec.js" "$out/fs.js"
list=$(printf '"%s",' $modules)
sed -e "s|__TITLE__|$title|" -e "s|__MODULES__|[${list%,}]|" "$web/index.html" >"$out/index.html"
echo "built $out"
