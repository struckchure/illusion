# Illusion

[![CI](https://github.com/struckchure/illusion/actions/workflows/ci.yml/badge.svg)](https://github.com/struckchure/illusion/actions/workflows/ci.yml)

Illusion is a small Go library that makes a few good tools work together for
making games: [raylib](https://www.raylib.com) for windowing, input, rendering
and audio, [Ark](https://github.com/mlange-42/ark) for the ECS, and
[Jolt Physics](https://github.com/jrouwe/JoltPhysics) for physics. The way it
fits them together is borrowed from [Bevy](https://bevyengine.org): your game
is data in an ECS, and systems are plain functions that ask for what they need.

It isn't a game engine. There's no editor, no scene format and no renderer of
its own; raylib, Ark and Jolt do the real work, and their types are used
directly (`rl.Vector3`, `ecs.Entity`, `*ecs.World`), so you can always drop
down to the library underneath.

```go
func main() {
	illusion.New().
		AddPlugins(defaults.Plugins(defaults.Config{})).
		AddSystems(illusion.Startup, illusion.Fn3(setup)).
		AddSystems(illusion.Update, illusion.Fn2(spin)).
		Run()
}

type Spin struct{ Speed float32 }

func setup(
	cmd *illusion.Commands,
	meshes *illusion.Res[asset.Assets[render.Mesh]],
	materials *illusion.Res[asset.Assets[render.StandardMaterial]],
) {
	cmd.Spawn(
		illusion.C(render.Camera3d{}),
		illusion.C(transform.FromXYZ(4, 3, 6).LookingAt(rl.Vector3{}, transform.Up)),
	)
	cmd.Spawn(
		illusion.C(render.DirectionalLight{Color: rl.White}),
		illusion.C(transform.Identity().LookingAt(rl.Vector3{X: -1, Y: -3, Z: -2}, transform.Up)),
	)
	cmd.Spawn(
		illusion.C(render.Mesh3d{Mesh: meshes.Get().Add(render.Cuboid(2, 2, 2))}),
		illusion.C(render.MeshMaterial3d{Material: materials.Get().Add(render.StandardMaterial{BaseColor: rl.Orange})}),
		illusion.C(transform.Identity()),
		illusion.C(Spin{Speed: 1}),
	)
}

func spin(q *illusion.Query2[Spin, transform.Transform], t *illusion.Res[illusion.Time]) {
	dt := t.Get().DeltaSecs()
	q.Each(func(_ ecs.Entity, s *Spin, tr *transform.Transform) {
		tr.RotateY(s.Speed * dt)
	})
}
```

## Requirements

- Go 1.25 or newer.
- A C and C++ toolchain for cgo. raylib and Jolt are compiled from source the
  first time you build (Jolt takes about half a minute; later builds are
  cached).
  - macOS: Xcode Command Line Tools (`xcode-select --install`).
  - Linux: gcc/g++ plus raylib's X11/Wayland/OpenGL headers, e.g. on
    Debian/Ubuntu `libgl1-mesa-dev libx11-dev libxi-dev libxcursor-dev
    libxrandr-dev libxinerama-dev libwayland-dev libxkbcommon-dev`.
  - Windows: MinGW-w64 gcc/g++ on PATH (e.g. from [MSYS2](https://www.msys2.org)
    or [WinLibs](https://winlibs.com)). The C++ runtime is linked statically,
    so binaries don't need MinGW's DLLs.

CI builds and tests on Linux, macOS and Windows; games have been played on
macOS (Apple Silicon).

```bash
go get github.com/struckchure/illusion
```

### Starting a game

`templates/game` is a [scaffold](https://hay-kot.github.io/scaffold/) template
for a new game project. It asks for a module path and a 3D or 2D starter
(3D can add physics), and sets up a Makefile that runs the game on the
desktop or builds it for the browser from the same source, plus, if you want
it, a GitHub Actions workflow that builds both:

```bash
go install github.com/hay-kot/scaffold@latest
scaffold new https://github.com/struckchure/illusion#templates/game
cd my-game && make run    # or: make serve
```

`templates/test.sh` generates every variant against a checkout and builds
each one for both platforms.

## Concepts

### App and plugins

An `illusion.App` holds the Ark world, the schedules and the plugins. A plugin
is anything with `Build(*illusion.App)`; it adds systems, resources and other
plugins. `defaults.Plugins` bundles the window, assets, input,
transforms, rendering, audio and diagnostics. Physics is a separate
`physics.Plugin`, so games without it skip the C++ build.

### Systems

Systems are plain functions. Wrap them with `Fn0` … `Fn8` (the number is how
many parameters they take); Go infers the rest, and there is no reflection:

```go
app.AddSystems(illusion.Update, illusion.Fn2(move))

func move(q *illusion.Query2[Position, Velocity], t *illusion.Res[illusion.Time]) { ... }
```

Parameters are initialized once and reused every run:

| Parameter | What it gives you |
|---|---|
| `*Query1[A]` … `*Query8[...]` | Entities with those components: `Each`, `Iter` (Ark's native loop), `Get`, `Single`, `Count`, `Contains` |
| `*Query2Where[A, B, F]` … | The same, filtered by type: `With[T]`, `Without[T]`, `And[F1, F2]`; `Query0Where[F]` for entities only |
| `*Res[T]` | A resource |
| `*Local[T]` | State private to the system |
| `*Commands` | Deferred spawn / insert / remove / despawn, safe inside query loops |
| `*EventReader[T]`, `*EventWriter[T]` | Events |
| `*Hierarchy` | Parent/child lookups |
| `*World` | The Ark world, for anything else |
| `*asset.Loader[T]`, `*physics.Physics`, `*audio.Audio`, `*diag.Gizmos` | Plugin APIs |

Write your own by implementing `InitParam(*ecs.World)` on a struct. When a
system needs more setup than parameters allow, implement `illusion.System`
(`Init` and `Run`) on a struct and add it with `illusion.Sys(...)`.

### Scheduling

Each frame runs `First → PreUpdate → StateTransition → Fixed* (zero or more
times, 60 Hz by default) → Update → PostUpdate → Render → Last`, after
`PreStartup → Startup → PostStartup` on the first frame. Inside a schedule,
order systems with `.After(...)`, `.Before(...)`, `illusion.Chain(...)` and
sets (`.InSet(...)`, `app.ConfigureSets(...)`), and gate them with
`.RunIf(...)`. Build conditions from functions with `Cond0` … `Cond8`, or use
the built-ins: `InState`, `StateChanged`, `ResourceExists`, `Once`, `Not`.

### Commands and hierarchy

```go
cmd.Spawn(illusion.C(Player{}), illusion.C(transform.FromXYZ(0, 1, 0))).
	WithChildren(func(c *illusion.ChildBuilder) {
		c.Spawn(illusion.C(render.Model3d{Model: bee}), illusion.C(transform.Identity()))
	})
cmd.Despawn(e) // takes the children with it
```

Children are linked with the `illusion.ChildOf` Ark relation, and their
`GlobalTransform` follows their parent's.

### Events and states

```go
func score(hits *illusion.EventReader[Hit]) {
	for hit := range hits.Read() { ... }
}

illusion.AddState(app, Menu)
app.AddSystems(illusion.OnEnter(InGame), illusion.Fn1(spawnLevel))
app.AddSystems(illusion.Update, illusion.Fn1(play).RunIf(illusion.InState(InGame)))
// elsewhere: next.Get().Set(InGame) with next *illusion.Res[illusion.NextState[AppState]]
```

Every reader sees each event exactly once. Entities with
`illusion.DespawnOnExit[S]{State: s}` are removed when the app leaves `s`.

## Plugins

Each plugin is a thin layer over one library: it registers the resources and
systems that connect it to the ECS, and otherwise gets out of the way.

| Package | What it does |
|---|---|
| `window` | Opens the raylib window and runs the main loop on the main thread |
| `input` | `Keys`, `MouseButtons` and `Mouse` resources (pressed / just pressed / just released); `Settings{Manual}` for tests and replays |
| `transform` | `Transform` and `GlobalTransform`, propagated through the hierarchy |
| `asset` | `Assets[T]` stores, typed handles, and reference-counted file loading |
| `render` | 3D: `Camera3d`, `Mesh3d`, `Model3d`, `StandardMaterial`, `DirectionalLight`. 2D: `Camera2d`, `Sprite`, `SpriteAnimation`, `Text2d`. Draw sets for immediate-mode raylib calls |
| `audio` | `Sound` and `Music` assets, the `Audio` parameter, and tone synthesis |
| `diag` | `Gizmos` (debug lines, boxes, spheres, capsules…) and a stats overlay (F3) |
| `physics` | Jolt rigid bodies, colliders, sensors, character controllers, raycasts, collision events; `DebugPlugin` draws colliders (F4) |
| `defaults` | `Plugins`, the bundle of the plugins above except physics |

## Examples

Run them from the repository root:

```bash
go run ./examples/cube
```

| Example | Shows |
|---|---|
| `cube` | The smallest 3D scene: a spinning, lit cube |
| `states` | A menu, a timed game and a pause screen with states and events |
| `bee` | A physics garden: character controller, dynamic bodies, collision events, raycasts, a loaded model, hierarchy, sounds, collider debug view |
| `sprites` | 2D: sprites, a generated sprite-sheet animation, a 2D camera, text, gizmos, sound |
| `physics` | A platformer playground built from the models in `examples/assets` |

### In the browser (experimental)

Any example can also be built for the web from the same source. It needs
[emscripten](https://emscripten.org) (`brew install emscripten`):

```bash
web/build.sh -a examples/assets ./examples/physics
python3 -m http.server -d build/web/physics 8080
```

Go can't use cgo when targeting wasm, so raylib and Jolt are built with
emscripten as separate wasm modules, and Go calls them through `syscall/js`.
`-a` bundles an asset directory into the page. Every example runs in the
browser, sound included. See [web/README.md](web/README.md).

## Repository layout

```
*.go            core: App, schedules, systems, queries, commands, events, states
asset/ audio/ defaults/ diag/ input/ physics/ render/ transform/ window/
internal/gen    generates the Query and Fn/Cond arity variants (go generate)
internal/jolt   cgo binding to Jolt; the vendored source lives in third_party
web/            browser builds: build and test scripts, raylib-go for the web
templates/game  scaffold template for new game projects
examples/
```

Run the tests with `go test ./...`; they don't need a window. After changing
`internal/gen`, run `go generate ./...`. To move to another Jolt release, run
`internal/jolt/update.sh <tag>`.

## Limitations

- One active 3D camera and one directional light; no shadows yet.
- Physics bodies should be root entities, and colliders ignore
  `Transform.Scale`. Rendering isn't interpolated between fixed steps.
- Systems run on one thread (raylib must stay on the main thread).
- Handles are reference-counted by hand: `Loader.Release` drops a reference.

## License

Illusion is licensed under the [MIT License](LICENSE). The vendored Jolt
Physics source in `internal/jolt/third_party` keeps its own MIT license
(`internal/jolt/third_party/JoltPhysics/LICENSE`).
