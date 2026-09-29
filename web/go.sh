#!/bin/sh
# Runs a go command for the browser (GOOS=js GOARCH=wasm), with raylib-go
# replaced by web/raylib, e.g. to vet or type-check web builds:
#
#   web/go.sh [-m module-dir] vet ./...
set -eu

web=$(cd "$(dirname "$0")" && pwd)
mod=$(cd "$web/.." && pwd)
if [ "${1:-}" = -m ]; then
	mod=$(cd "$2" && pwd)
	shift 2
fi
. "$web/lib.sh"

cd "$mod"
GOOS=js GOARCH=wasm go "$@"
