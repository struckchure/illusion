#!/bin/sh
# Generates every variant of the game template against this checkout of
# illusion and builds each one for the desktop and the browser. Needs scaffold
# (go install github.com/hay-kot/scaffold@latest) and emscripten.
#
#   templates/test.sh [out-dir]
#
# The projects are left in out-dir (a temporary directory by default) to try
# out: cd there, then make run or make serve.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
out=${1:-$(mktemp -d)}
mkdir -p "$out"
out=$(cd "$out" && pwd)

# name|kind|physics
for v in "Crate Pusher|3D|true" "Cube Walk|3D|false" "Coin Dash|2D|false"; do
	name=${v%%|*} rest=${v#*|}
	kind=${rest%%|*} physics=${rest#*|}
	dir=$(echo "$name" | tr 'A-Z ' 'a-z-')
	echo "== $name ($kind, physics=$physics)"
	rm -rf "${out:?}/$dir"
	scaffold --log-level=error --run-hooks=always new --no-prompt --output-dir "$out" \
		"$root/templates/game" "Project=$name" "module=example.com/$dir" \
		"kind=$kind" "physics:bool=$physics" "illusion_path=$root"
	(
		cd "$out/$dir"
		test -z "$(gofmt -l .)" || { echo "$dir: not gofmt'd" >&2; exit 1; }
		go vet ./...
		make build web
	)
done
echo "all variants built in $out"
