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

web=$(cd "$(dirname "$0")" && pwd)
mod=$(cd "$web/.." && pwd)
. "$web/lib.sh"

# Packages come first; everything from the first flag on goes to go test.
pkgs=
while [ $# -gt 0 ] && [ "${1#-}" = "$1" ]; do
	pkgs="$pkgs $1"
	shift
done

modules=$work/modules
mkdir -p "$modules"
if uses_jolt $pkgs; then
	build_jolt "$modules" node
fi

export ILLUSION_WASM_MODULES=$modules
cd "$mod"
GOOS=js GOARCH=wasm go test -exec "$web/testexec.sh" $pkgs "$@"
