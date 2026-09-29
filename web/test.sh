#!/bin/sh
# Runs Go tests compiled to wasm under Node, with the C modules they need
# loaded, to check the browser bindings against the same tests as native:
#
#   web/test.sh <packages> [go test flags]
#   web/test.sh ./internal/jolt ./physics -run Contact -v
#
# Packages that draw need a browser; this is for everything else. raylib's
# types and math work without raylib loaded.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
web=$root/web
. "$web/lib.sh"

# Packages come first; everything from the first flag on goes to go test.
pkgs=
while [ $# -gt 0 ] && [ "${1#-}" = "$1" ]; do
	pkgs="$pkgs $1"
	shift
done

modules=$cache/node
mkdir -p "$modules"
rm -f "$modules"/*.js "$modules"/*.wasm
if uses_jolt $pkgs; then
	build_jolt "$modules" node
fi

export ILLUSION_WASM_MODULES=$modules
cd "$root"
GOOS=js GOARCH=wasm go test -exec "$web/testexec.sh" $pkgs "$@"
