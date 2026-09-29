#!/bin/sh
# Resolves the game's dependencies (illusion, raylib-go, Ark) and formats the
# generated code. Runs in the output directory, which holds the project.
set -e
cd "{{ .ProjectKebab }}"
if ! go mod tidy; then
	echo "go mod tidy failed; run it in {{ .ProjectKebab }} once you're online" >&2
	exit 0
fi
gofmt -w .
