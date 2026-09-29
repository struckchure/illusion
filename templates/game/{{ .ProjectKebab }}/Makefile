# Desktop: make run, make build. Browser: make web (or make serve to build and
# serve it). Web builds need emscripten (emcc) on PATH.

GAME := [[ .ProjectKebab ]]
TITLE := [[ .Project ]]
PORT ?= 8080

# illusion's directory: the module cache, or a local checkout if go.mod
# replaces it. Its web/build.sh does the browser build.
ILLUSION = $(shell go list -m -f '{{.Dir}}' github.com/struckchure/illusion)

.PHONY: run build web serve test clean

run:
	go run .

build:
	go build -o build/$(GAME)$(shell go env GOEXE) .

web:
	sh "$(ILLUSION)/web/build.sh" -m . -o build/web -a assets -t "$(TITLE)" .

serve: web
	@echo "serving on http://localhost:$(PORT)"
	python3 -m http.server -d build/web $(PORT)

test:
	go test ./...

clean:
	rm -rf build
