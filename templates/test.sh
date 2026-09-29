#!/bin/sh
# Generates every variant of the game template against this checkout of
# illusion and builds each one for the desktop and the browser. Needs scaffold
# (go install github.com/hay-kot/scaffold@latest) and emscripten. Generated
# GitHub workflows are also checked with actionlint, if it's installed.
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

# name|kind|physics|ci
for v in "Crate Pusher|3D|true|true" "Cube Walk|3D|false|false" "Coin Dash|2D|false|true"; do
	name=${v%%|*} rest=${v#*|}
	kind=${rest%%|*} rest=${rest#*|}
	physics=${rest%%|*} ci=${rest#*|}
	dir=$(echo "$name" | tr 'A-Z ' 'a-z-')
	echo "== $name ($kind, physics=$physics, ci=$ci)"
	rm -rf "${out:?}/$dir"
	scaffold --log-level=error --run-hooks=always new --no-prompt --output-dir "$out" \
		"$root/templates/game" "Project=$name" "module=example.com/$dir" \
		"kind=$kind" "physics:bool=$physics" "ci:bool=$ci" "illusion_path=$root"
	(
		cd "$out/$dir"
		test -z "$(gofmt -l .)" || { echo "$dir: not gofmt'd" >&2; exit 1; }
		if [ "$ci" = true ]; then
			test -f .github/workflows/ci.yml || { echo "$dir: no workflow" >&2; exit 1; }
			if command -v actionlint >/dev/null; then actionlint .github/workflows/ci.yml; fi
		else
			test ! -e .github || { echo "$dir: unexpected .github" >&2; exit 1; }
		fi
		go vet ./...
		make build web
	)
done
echo "all variants built in $out"
