# Browser builds

```bash
web/build.sh ./examples/cube                       # writes build/web/cube/
web/build.sh -a examples/assets ./examples/bee     # bundles the assets it loads
go run ./web/serve -dir build/web/bee              # http://localhost:8080
web/test.sh ./internal/jolt ./physics              # the usual tests, as wasm under Node
web/go.sh vet ./...                                # any go command, for the browser
```

The scripts need [emscripten](https://emscripten.org) (`emcc`) on PATH. The
first build compiles raylib and Jolt, which takes about half a minute. The
objects are then cached in `~/.cache/illusion/web` (set `ILLUSION_WEB_CACHE`
to move it).

Games in their own module use the same scripts from illusion's directory in
the module cache. `-m` names the game's module:

```bash
sh "$(go list -m -f '{{.Dir}}' github.com/struckchure/illusion)/web/build.sh" -m . -a assets .
```

Projects made from the game template (`templates/game`) wrap this in
`make web`.

## How it fits together

Go can't use cgo when it targets `js/wasm`, so every C library runs as its
own emscripten module next to Go's:

```
game.wasm    Go: your game, illusion, Ark
  │ syscall/js
  ├─▶ raylib.wasm  raylib + raylib/glue.c
  └─▶ jolt.wasm    Jolt + internal/jolt/glue.cpp, only when the game uses physics
```

The page (`index.html`) loads each module into `globalThis.<name>` and then
starts Go. The modules don't share memory. Pointer arguments are copied
through a scratch block in the C module's heap, and results are read back
from there (see `internal/emscripten`).

- Each script builds in a temporary Go workspace (`lib.sh`) that uses the
  module being built unchanged and replaces raylib-go with `raylib/`, so game
  code is the same for desktop and web.
- `raylib/` has raylib-go's API, implemented over `raylib.wasm`. Every file
  in it builds only for `js`, and it has no `go.mod`: a nested module would be
  left out of illusion's module download. The workspace gets a copy with a
  `go.mod` naming it raylib-go. `sync.sh` copies raylib-go's pure-Go types
  and math into it. Re-run it after bumping raylib-go.
- `internal/jolt/jolt_js.go` is the same binding as the cgo one, implemented
  over `jolt.wasm`. Jolt runs single-threaded with wasm SIMD.
- `lib.sh` builds the modules, and `build.sh`, `test.sh` and `go.sh` drive
  it. Illusion may be read-only in the module cache, so nothing is written
  under `web/`.
  `testexec.cjs` runs a Go test binary in Node with the modules loaded.
- Files: `-a dir` bundles a directory into raylib's in-memory filesystem
  at the same relative path (as `raylib.data`), so the game's paths work
  unchanged. `fs.js` implements the Node-style `fs` that Go's wasm runtime
  uses on top of that filesystem. Go's `os` package (the asset loader, the
  OBJ fix-up's temp files) and raylib's loaders see the same files. Writes
  last until the page closes.
- Meshes and models: raylib keeps them. The Go `rl.Mesh` holds the counts,
  VAO and VBO ids, and for generated and loaded meshes Go copies of the
  positions, texcoords, normals and indices (physics colliders read these).
  A loaded `rl.Model` mirrors its meshes, materials and mesh-material table.
  Meshes built in Go keep their own arrays: `UploadMesh` copies them to
  raylib, `UpdateMeshBuffer` updates the GPU, and functions that read the
  arrays (`GetMeshBoundingBox`, `GenMeshTangents`, `ExportMesh`) copy the Go
  arrays over raylib's first. On desktop, raylib reads the Go arrays directly,
  so both see the same data.
- Skeletal animation runs in raylib, skinning on the CPU as desktop raylib
  does by default. The Go `rl.Model` mirrors its skeleton, and its
  `CurrentPose` and `BoneMatrices` are re-read after each
  `UpdateModelAnimation`. `rl.ModelAnimation` mirrors every keyframe pose.
  `DrawModel`/`DrawModelEx` run on the Go side, so changes to the Go
  materials show up. The animated vertices stay in raylib.
- Fonts: raylib keeps them. The Go `rl.Font` mirrors the glyph rectangles and
  metrics (not the glyph images). `LoadFontFromMemory` works with `go:embed`.
- Images live in Go memory. `ImageDraw*` functions run raylib's own code on
  a copy of the pixels, so they match desktop exactly.
- Audio: sounds and music stay in raylib's heap. Their Go values carry a Go
  placeholder in `Stream.Buffer` that keys the C pointer (a fake pointer
  would confuse Go's garbage collector). Browsers start audio suspended; the
  page resumes it on the first click, touch or key press.
- Platform-specific pieces in illusion itself: the main loop runs from
  `requestAnimationFrame` (`window/loop_js.go`), and shaders are GLSL ES 3.00
  (`render/shader_js.go`).

## The canvas

By default the canvas is `window.Config`'s Width x Height, centered on the
page. With `Resizable` it fills the page and follows it. With `HighDPI` its
drawing buffer is its CSS size times `devicePixelRatio`, so it's sharp on
Retina screens and phones. Screen coordinates are then device pixels, so
scale 2D sizes by `window.Window.Scale`. The window package sizes the canvas
itself, every frame, rather than leaving it to raylib, whose web resize
handler ignores `devicePixelRatio`. It sets the CSS size with a stylesheet
rule, since emscripten clears the canvas's inline size whenever the buffer
changes.

## Not yet

- raylib coverage: only the functions illusion and its examples use, plus
  a few common drawing calls. Anything else fails to compile with
  "undefined: rl.X". Add it to `raylib/` (and to `glue.c` and `lib.sh` if it
  takes structs or isn't exported yet).
- `DrawModelWires`, instancing (`DrawMeshInstanced`), and the other model
  and mesh functions nothing in the repo uses yet.
- `WindowShouldClose` is always false and `SetTargetFPS` does nothing.
  `requestAnimationFrame` paces frames.
