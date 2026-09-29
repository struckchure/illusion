#!/bin/sh
# go test -exec helper for web/test.sh (see testexec.cjs).
exec node --stack-size=8192 "$(dirname "$0")/testexec.cjs" "$@"
