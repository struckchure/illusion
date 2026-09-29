// Coin rush: a menu, a timed game and a pause screen, built from states and
// events.
//
// Menu: Enter or click Play to start, Q to quit. In game: WASD to move,
// Escape to pause or resume, collect every coin before time runs out.
package main

import (
	"fmt"
	"math"
	"math/rand/v2"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/defaults"
	"github.com/struckchure/illusion/input"
	"github.com/struckchure/illusion/render"
	"github.com/struckchure/illusion/transform"
	"github.com/struckchure/illusion/window"
)

// AppState is the top-level screen.
type AppState int

const (
	Menu AppState = iota
	InGame
)

func (s AppState) String() string { return [...]string{"Menu", "InGame"}[s] }

// Phase is only meaningful in game. It's a separate state type so pausing
// doesn't leave InGame (which would despawn the level).
type Phase int

const (
	Running Phase = iota
	Paused
)

func (p Phase) String() string { return [...]string{"Running", "Paused"}[p] }

const (
	coinCount  = 12
	roundTime  = 30.0
	arenaSize  = 9.0
	moveSpeed  = 7.0
	pickupDist = 0.9
)

// Gameplay systems only run in game and while not paused.
const Gameplay illusion.SystemSet = "gameplay"

type Player struct{}
type Coin struct{}
type Spinner struct{ Speed float32 }
type Popup struct{ Age float32 }

// CoinCollected is sent when the player picks up a coin.
type CoinCollected struct{ Position rl.Vector3 }

// Round is the current game's progress.
type Round struct {
	Collected int
	TimeLeft  float32
}

// Results carries the outcome of the last round back to the menu.
type Results struct {
	Last string
	Best int
}

// Palette holds the meshes and materials the game spawns with.
type Palette struct {
	Cube, Coin, Popup  asset.Handle[render.Mesh]
	Player, Gold, Menu asset.Handle[render.StandardMaterial]
}

func main() {
	app := illusion.New().
		AddPlugins(defaults.Plugins(defaults.Config{
			Window: window.Config{Title: "Illusion: coin rush", MSAA: true, KeepEscape: true},
		})).
		InsertResource(
			illusion.R(&render.ClearColor{Color: rl.NewColor(28, 30, 44, 255)}),
			illusion.R(&Results{}),
			illusion.R(&Round{}),
		)
	illusion.AddState(app, Menu)
	illusion.AddState(app, Running)

	app.
		AddSystems(illusion.Startup, illusion.Fn3(setup)).
		// Menu
		AddSystems(illusion.OnEnter(Menu), illusion.Fn2(spawnMenuScene)).
		AddSystems(illusion.Update, illusion.Fn6(menuInput).RunIf(illusion.InState(Menu))).
		AddSystems(illusion.Render, illusion.Fn2(drawMenu).InSet(render.Draw2D).RunIf(illusion.InState(Menu))).
		// Game
		AddSystems(illusion.OnEnter(InGame), illusion.Fn4(startRound)).
		ConfigureSets(illusion.Update, Gameplay.RunIf(illusion.InState(InGame), illusion.InState(Running))).
		AddSystems(illusion.Update,
			illusion.Fn3(pauseInput).RunIf(illusion.InState(InGame)),
			illusion.Chain(illusion.Fn3(movePlayer), collectCoins).InSet(Gameplay),
			illusion.Fn4(keepScore).InSet(Gameplay).After(collectCoins),
			illusion.Fn3(spawnPopups).InSet(Gameplay).After(collectCoins),
			illusion.Fn3(animatePopups).InSet(Gameplay),
		).
		AddSystems(illusion.Update, illusion.Fn2(spin)). // decoration keeps spinning even when paused
		AddSystems(illusion.Render,
			illusion.Fn1(drawHUD).InSet(render.Draw2D).RunIf(illusion.InState(InGame)),
			illusion.Fn1(drawPaused).InSet(render.Draw2D).RunIf(illusion.InState(InGame), illusion.InState(Paused)),
		).
		Run()
}

// collectCoins is kept in a variable so other systems can be ordered after it.
var collectCoins = illusion.Fn4(collect)

func setup(
	cmd *illusion.Commands,
	meshes *illusion.Res[asset.Assets[render.Mesh]],
	materials *illusion.Res[asset.Assets[render.StandardMaterial]],
) {
	m, mat := meshes.Get(), materials.Get()
	cmd.InsertResource(illusion.R(&Palette{
		Cube:   m.Add(render.Cuboid(1, 1, 1)),
		Coin:   m.Add(render.Cylinder(0.4, 0.1)),
		Popup:  m.Add(render.Sphere(0.15)),
		Player: mat.Add(render.StandardMaterial{BaseColor: rl.SkyBlue}),
		Gold:   mat.Add(render.StandardMaterial{BaseColor: rl.Gold}),
		Menu:   mat.Add(render.StandardMaterial{BaseColor: rl.Violet}),
	}))

	cmd.Spawn(
		illusion.C(render.Camera3d{Fovy: 50}),
		illusion.C(transform.FromXYZ(0, 16, 13).LookingAt(rl.Vector3{}, transform.Up)),
	)
	cmd.Spawn(
		illusion.C(render.DirectionalLight{Color: rl.White}),
		illusion.C(transform.Identity().LookingAt(rl.Vector3{X: -1, Y: -3, Z: -1}, transform.Up)),
	)
	cmd.Spawn(
		illusion.C(render.Mesh3d{Mesh: m.Add(render.Plane(2*arenaSize+2, 2*arenaSize+2))}),
		illusion.C(render.MeshMaterial3d{Material: mat.Add(render.StandardMaterial{BaseColor: rl.NewColor(60, 64, 90, 255)})}),
		illusion.C(transform.Identity()),
	)
}

// --- Menu -------------------------------------------------------------------

func spawnMenuScene(cmd *illusion.Commands, p *illusion.Res[Palette]) {
	palette := p.Get()
	cmd.Spawn(
		illusion.C(render.Mesh3d{Mesh: palette.Cube}),
		illusion.C(render.MeshMaterial3d{Material: palette.Menu}),
		illusion.C(transform.FromXYZ(0, 2, 0).WithScale(3)),
		illusion.C(Spinner{Speed: 0.8}),
		illusion.C(illusion.DespawnOnExit[AppState]{State: Menu}),
	)
}

func playButton(win *window.Window) rl.Rectangle {
	return rl.Rectangle{X: float32(win.Width)/2 - 100, Y: float32(win.Height)/2 + 40, Width: 200, Height: 56}
}

func menuInput(
	keys *illusion.Res[input.Keys],
	buttons *illusion.Res[input.MouseButtons],
	mouse *illusion.Res[input.Mouse],
	win *illusion.Res[window.Window],
	next *illusion.Res[illusion.NextState[AppState]],
	cmd *illusion.Commands,
) {
	clicked := buttons.Get().JustPressed(rl.MouseButtonLeft) &&
		rl.CheckCollisionPointRec(mouse.Get().Position, playButton(win.Get()))
	if clicked || keys.Get().JustPressed(rl.KeyEnter) {
		next.Get().Set(InGame)
	}
	if keys.Get().JustPressed(rl.KeyQ) {
		cmd.Exit()
	}
}

func drawMenu(win *illusion.Res[window.Window], results *illusion.Res[Results]) {
	w := win.Get()
	centered("COIN RUSH", w.Height/2-120, 64, rl.Gold, w)
	centered(fmt.Sprintf("collect %d coins in %d seconds", coinCount, int(roundTime)), w.Height/2-40, 22, rl.RayWhite, w)

	btn := playButton(w)
	hover := rl.CheckCollisionPointRec(rl.GetMousePosition(), btn)
	fill := rl.NewColor(70, 70, 110, 255)
	if hover {
		fill = rl.NewColor(100, 100, 160, 255)
	}
	rl.DrawRectangleRounded(btn, 0.3, 8, fill)
	centered("PLAY", int(btn.Y)+14, 28, rl.RayWhite, w)
	centered("enter: play    q: quit", int(btn.Y+btn.Height)+20, 18, rl.LightGray, w)

	r := results.Get()
	if r.Last != "" {
		centered(fmt.Sprintf("%s   best: %d", r.Last, r.Best), w.Height-60, 22, rl.SkyBlue, w)
	}
}

// --- Game -------------------------------------------------------------------

func startRound(
	cmd *illusion.Commands,
	p *illusion.Res[Palette],
	round *illusion.Res[Round],
	phase *illusion.Res[illusion.NextState[Phase]],
) {
	*round.Get() = Round{TimeLeft: roundTime}
	phase.Get().Set(Running)

	palette := p.Get()
	scoped := illusion.C(illusion.DespawnOnExit[AppState]{State: InGame})
	cmd.Spawn(
		illusion.C(Player{}),
		illusion.C(render.Mesh3d{Mesh: palette.Cube}),
		illusion.C(render.MeshMaterial3d{Material: palette.Player}),
		illusion.C(transform.FromXYZ(0, 0.5, 0)),
		scoped,
	)
	for range coinCount {
		x := (rand.Float32()*2 - 1) * arenaSize
		z := (rand.Float32()*2 - 1) * arenaSize
		cmd.Spawn(
			illusion.C(Coin{}),
			illusion.C(Spinner{Speed: 3}),
			illusion.C(render.Mesh3d{Mesh: palette.Coin}),
			illusion.C(render.MeshMaterial3d{Material: palette.Gold}),
			illusion.C(transform.FromXYZ(x, 0.6, z).WithRotation(rl.QuaternionFromAxisAngle(transform.Right, math.Pi/2))),
			scoped,
		)
	}
}

func pauseInput(
	keys *illusion.Res[input.Keys],
	phase *illusion.Res[illusion.State[Phase]],
	next *illusion.Res[illusion.NextState[Phase]],
) {
	if !keys.Get().JustPressed(rl.KeyEscape) {
		return
	}
	if phase.Get().Get() == Running {
		next.Get().Set(Paused)
	} else {
		next.Get().Set(Running)
	}
}

func movePlayer(
	q *illusion.Query1Where[transform.Transform, illusion.With[Player]],
	keys *illusion.Res[input.Keys],
	t *illusion.Res[illusion.Time],
) {
	k := keys.Get()
	var dir rl.Vector3
	if k.AnyPressed(rl.KeyW, rl.KeyUp) {
		dir.Z--
	}
	if k.AnyPressed(rl.KeyS, rl.KeyDown) {
		dir.Z++
	}
	if k.AnyPressed(rl.KeyA, rl.KeyLeft) {
		dir.X--
	}
	if k.AnyPressed(rl.KeyD, rl.KeyRight) {
		dir.X++
	}
	if rl.Vector3LengthSqr(dir) == 0 {
		return
	}
	step := rl.Vector3Scale(rl.Vector3Normalize(dir), moveSpeed*t.Get().DeltaSecs())
	q.Each(func(_ ecs.Entity, tr *transform.Transform) {
		tr.Translation = rl.Vector3Add(tr.Translation, step)
		tr.Translation.X = rl.Clamp(tr.Translation.X, -arenaSize, arenaSize)
		tr.Translation.Z = rl.Clamp(tr.Translation.Z, -arenaSize, arenaSize)
	})
}

// collect despawns coins the player touches and announces each pickup.
func collect(
	players *illusion.Query1Where[transform.Transform, illusion.With[Player]],
	coins *illusion.Query1Where[transform.Transform, illusion.With[Coin]],
	collected *illusion.EventWriter[CoinCollected],
	cmd *illusion.Commands,
) {
	_, player, ok := players.Single()
	if !ok {
		return
	}
	coins.Each(func(e ecs.Entity, coin *transform.Transform) {
		if rl.Vector3Distance(player.Translation, coin.Translation) < pickupDist {
			cmd.Despawn(e)
			collected.Send(CoinCollected{Position: coin.Translation})
		}
	})
}

// keepScore counts pickups and ends the round on a win or when time runs out.
func keepScore(
	collected *illusion.EventReader[CoinCollected],
	round *illusion.Res[Round],
	results *illusion.Res[Results],
	ctx *roundContext,
) {
	r := round.Get()
	r.Collected += collected.Len()
	collected.Clear()
	r.TimeLeft -= ctx.time.Get().DeltaSecs()

	res := results.Get()
	switch {
	case r.Collected >= coinCount:
		res.Last = fmt.Sprintf("all %d coins with %.1fs to spare!", coinCount, r.TimeLeft)
	case r.TimeLeft <= 0:
		res.Last = fmt.Sprintf("time's up: %d of %d coins", r.Collected, coinCount)
	default:
		return
	}
	res.Best = max(res.Best, r.Collected)
	ctx.next.Get().Set(Menu)
}

// roundContext bundles parameters keepScore needs; custom params are just
// structs that initialize their fields.
type roundContext struct {
	time illusion.Res[illusion.Time]
	next illusion.Res[illusion.NextState[AppState]]
}

func (c *roundContext) InitParam(w *ecs.World) {
	c.time.InitParam(w)
	c.next.InitParam(w)
}

// spawnPopups reads the same events as keepScore; each reader gets every event.
func spawnPopups(collected *illusion.EventReader[CoinCollected], p *illusion.Res[Palette], cmd *illusion.Commands) {
	palette := p.Get()
	for ev := range collected.Read() {
		cmd.Spawn(
			illusion.C(Popup{}),
			illusion.C(render.Mesh3d{Mesh: palette.Popup}),
			illusion.C(render.MeshMaterial3d{Material: palette.Gold}),
			illusion.C(transform.FromTranslation(ev.Position)),
			illusion.C(illusion.DespawnOnExit[AppState]{State: InGame}),
		)
	}
}

func animatePopups(q *illusion.Query2[Popup, transform.Transform], t *illusion.Res[illusion.Time], cmd *illusion.Commands) {
	dt := t.Get().DeltaSecs()
	q.Each(func(e ecs.Entity, p *Popup, tr *transform.Transform) {
		p.Age += dt
		tr.Translation.Y += 3 * dt
		tr.Scale = rl.Vector3Scale(rl.Vector3One(), max(0, 1-p.Age))
		if p.Age >= 1 {
			cmd.Despawn(e)
		}
	})
}

func spin(q *illusion.Query2[Spinner, transform.Transform], t *illusion.Res[illusion.Time]) {
	dt := t.Get().DeltaSecs()
	q.Each(func(_ ecs.Entity, s *Spinner, tr *transform.Transform) {
		tr.RotateY(s.Speed * dt)
	})
}

func drawHUD(round *illusion.Res[Round]) {
	r := round.Get()
	rl.DrawText(fmt.Sprintf("coins %d/%d", r.Collected, coinCount), 16, 16, 28, rl.Gold)
	rl.DrawText(fmt.Sprintf("time %.1f", max(0, r.TimeLeft)), 16, 50, 28, rl.RayWhite)
	rl.DrawText("esc: pause", 16, 84, 18, rl.LightGray)
}

func drawPaused(win *illusion.Res[window.Window]) {
	w := win.Get()
	rl.DrawRectangle(0, 0, int32(w.Width), int32(w.Height), rl.Fade(rl.Black, 0.55))
	centered("PAUSED", w.Height/2-40, 56, rl.RayWhite, w)
	centered("esc: resume", w.Height/2+30, 22, rl.LightGray, w)
}

func centered(text string, y, size int, c rl.Color, w *window.Window) {
	width := rl.MeasureText(text, int32(size))
	rl.DrawText(text, int32(w.Width)/2-width/2, int32(y), int32(size), c)
}
