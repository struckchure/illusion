# {{ .Project }}

A {{ .Scaffold.kind }} game built with [illusion](https://github.com/struckchure/illusion){{ if .Scaffold.physics }}, with Jolt physics{{ end }}.
The same code runs on the desktop and in the browser.

## Requirements

- Go 1.25 or newer.
- For the desktop: a C toolchain, since raylib compiles with cgo{{ if .Scaffold.physics }} (Jolt
  too; its first build takes about half a minute){{ end }}. On macOS, `xcode-select --install`;
  on Debian/Ubuntu, `gcc g++ libgl1-mesa-dev libxi-dev libxcursor-dev libxrandr-dev libxinerama-dev`.
- For the browser: [emscripten](https://emscripten.org) (`brew install emscripten`) and
  Python 3 to serve the page.

## Running

```sh
make run      # opens a desktop window
make serve    # builds for the browser and serves it on http://localhost:8080
```

`make build` writes a desktop binary to `build/`, and `make web` writes the
browser build to `build/web/` (static files you can host anywhere). The first
web build compiles raylib{{ if .Scaffold.physics }} and Jolt{{ end }} with emscripten, which takes a minute;
later builds take seconds.

## Layout

- `main.go` is the game: plugins, a startup system that spawns the scene, and
  update systems. See illusion's README for how systems, queries and
  resources work.
- `assets/` holds files the game loads by path. Desktop builds read them from
  disk, relative to the working directory, so run the game from here. Web
  builds bundle the directory into the page.

## Notes for the browser

- Everything in `assets/` is downloaded before the game starts, so keep it to
  what the game needs.
- Browsers keep sound off until the player clicks or presses a key.
- Web builds use a browser version of raylib-go that covers the functions
  illusion and its examples use. A raylib function it doesn't have yet fails
  the web build with `undefined: rl.X`.
