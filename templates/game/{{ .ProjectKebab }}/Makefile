# Desktop: make run, make build. Browser: make web (or make serve to build it
# and serve it with cmd/web). Web builds need emscripten (emcc) on PATH.

GAME := [[ .ProjectKebab ]]
TITLE := [[ .Project ]]
PORT ?= 8080

# illusion's directory: the module cache, or a local checkout if go.mod
# replaces it. Its web/build.sh does the browser build.
ILLUSION = $(shell go list -m -f '{{.Dir}}' github.com/struckchure/illusion)

.PHONY: run build web serve test clean

run:
	go run ./cmd/desktop

build:
	go build -o build/$(GAME)$(shell go env GOEXE) ./cmd/desktop

web:
	sh "$(ILLUSION)/web/build.sh" -m . -o build/web -a assets -t "$(TITLE)" ./cmd/web

serve: web
	go run ./cmd/web -dir build/web -addr :$(PORT)

test:
	go test ./...

clean:
	rm -rf build
