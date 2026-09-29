// Treasure island: a physics playground built from the platformer kit in
// examples/assets.
//
// The bee is a character controller. Knock over the crate stacks with bombs,
// ride the moving rock or the spring up to the floating island, grab the
// coins on the way, and find the treasure.
//
// WASD move, Space jump, E or left click throw a bomb, R respawn, Escape quit.
package main

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/defaults"
	"github.com/struckchure/illusion/input"
	"github.com/struckchure/illusion/physics"
	"github.com/struckchure/illusion/render"
	"github.com/struckchure/illusion/transform"
	"github.com/struckchure/illusion/window"
)

const (
	walkSpeed   = 5.0
	jumpSpeed   = 6.5
	springSpeed = 15.0
	bombSpeed   = 9.0
	blastRadius = 5.0
	blastPower  = 90.0
)

var spawnPoint = rl.Vector3{Y: 1.5, Z: 6}

// Player is the bee's physics root; its Body child is the model.
type Player struct{}
type Body struct{}

// Coin is a pickup: a sensor at the root, a spinning model as its child.
type Coin struct{}

// Spinner rotates an entity around its parent's Y axis.
type Spinner struct{ Speed float32 }

// Spring launches the player upward on contact.
type Spring struct{}

// Treasure wins the game on contact.
type Treasure struct{}

// Bomb explodes when its fuse runs out.
type Bomb struct{ Fuse float32 }

// Blast is the short-lived explosion flash.
type Blast struct{ Age float32 }

// Mover slides a kinematic body between two points.
type Mover struct {
	From, To rl.Vector3
	Period   float32 // seconds for a round trip
}

// Roller spins a kinematic body around its X axis.
type Roller struct{ Speed float32 }

// Drift floats decoration back and forth.
type Drift struct {
	Origin rl.Vector3
	Amount rl.Vector3
	Speed  float32
}

type FollowCamera struct{ Offset rl.Vector3 }

// Game tracks progress.
type Game struct {
	Coins, TotalCoins int
	Won               bool
}

// Kit is the shared material and the handles spawned at runtime.
type Kit struct {
	Material   render.MeshMaterial3d
	Bomb       asset.Handle[render.Model]
	BombHull   physics.Collider
	BlastMesh  asset.Handle[render.Mesh]
	BlastColor render.MeshMaterial3d
}

func main() {
	illusion.New().
		AddPlugins(
			defaults.Plugins(defaults.Config{
				Window:    window.Config{Title: "Illusion: treasure island", MSAA: true},
				AssetRoot: "examples/assets",
			}),
			physics.Plugin{},
		).
		InsertResource(
			illusion.R(&render.ClearColor{Color: rl.NewColor(142, 202, 240, 255)}),
			illusion.R(&render.AmbientLight{Color: rl.NewColor(200, 220, 255, 255), Brightness: 0.35}),
			illusion.R(&Game{}),
		).
		AddSystems(illusion.Startup, illusion.Fn5(setup)).
		// Kinematic motion runs at the physics rate so pushes are smooth.
		AddSystems(illusion.FixedUpdate, illusion.Fn2(moveMovers), illusion.Fn2(rollRollers)).
		AddSystems(illusion.Update,
			illusion.Chain(illusion.Fn3(walk), illusion.Fn2(follow)),
			illusion.Fn5(throwBomb),
			illusion.Fn5(fuses),
			illusion.Fn4(blastForces),
			illusion.Fn3(blasts),
			illusion.Fn5(touches),
			illusion.Fn2(respawn),
			illusion.Fn2(spin),
			illusion.Fn2(drift),
		).
		AddSystems(illusion.Render, illusion.Fn2(hud).InSet(render.Draw2D)).
		Run()
}

func setup(
	cmd *illusion.Commands,
	models *asset.Loader[render.Model],
	textures *asset.Loader[render.Texture],
	materials *illusion.Res[asset.Assets[render.StandardMaterial]],
	meshes *illusion.Res[asset.Assets[render.Mesh]],
) {
	// Every model in the kit is UV-mapped onto the same palette texture.
	kitMaterial := render.MeshMaterial3d{Material: materials.Get().Add(render.StandardMaterial{
		BaseColor: rl.White,
		Texture:   textures.MustLoad("palette.png"),
	})}
	load := func(file string) (asset.Handle[render.Model], rl.Model) {
		h := models.MustLoad(file)
		return h, models.Get(h).Model
	}
	// prop spawns a kit model at pos; extra adds components like colliders.
	prop := func(file string, tr transform.Transform, extra ...illusion.Component) illusion.EntityCommands {
		h, _ := load(file)
		return cmd.Spawn(append([]illusion.Component{
			illusion.C(render.Model3d{Model: h}),
			illusion.C(kitMaterial),
			illusion.C(tr),
		}, extra...)...)
	}
	solid := func(c physics.Collider) []illusion.Component {
		return []illusion.Component{illusion.C(physics.Static), illusion.C(c)}
	}
	at := transform.FromXYZ

	_, bombModel := load("bomb.obj")
	cmd.InsertResource(illusion.R(&Kit{
		Material:   kitMaterial,
		Bomb:       models.MustLoad("bomb.obj"),
		BombHull:   physics.ModelConvexHull(bombModel),
		BlastMesh:  meshes.Get().Add(render.Sphere(1)),
		BlastColor: render.MeshMaterial3d{Material: materials.Get().Add(render.StandardMaterial{BaseColor: rl.Orange, Unlit: true})},
	}))

	// Light and camera.
	cmd.Spawn(
		illusion.C(render.DirectionalLight{Color: rl.NewColor(255, 246, 225, 255)}),
		illusion.C(transform.Identity().LookingAt(rl.Vector3{X: -0.6, Y: -1, Z: -0.4}, transform.Up)),
	)
	cmd.Spawn(
		illusion.C(render.Camera3d{Fovy: 55}),
		illusion.C(FollowCamera{Offset: rl.Vector3{Y: 5, Z: 9}}),
		illusion.C(at(0, 6, 15).LookingAt(spawnPoint, transform.Up)),
	)

	// Main island: 3x3 grass tiles, 8m each, tops at y=0.
	_, grass8 := load("ground_grass_8.obj")
	for x := -8; x <= 8; x += 8 {
		for z := -8; z <= 8; z += 8 {
			prop("ground_grass_8.obj", at(float32(x), 0, float32(z)), solid(physics.ModelBox(grass8))...)
		}
	}
	// Water around it.
	for x := -16; x <= 16; x += 4 {
		for z := -16; z <= 16; z += 4 {
			if abs(x) > 12 || abs(z) > 12 {
				prop("water_4.obj", at(float32(x), -0.6, float32(z)))
			}
		}
	}

	// Floating island, 6m up, reached by the moving rock or the spring.
	_, grass4 := load("ground_grass_4.obj")
	for _, x := range []float32{19, 23} {
		for _, z := range []float32{-6, -2} {
			prop("ground_grass_4.obj", at(x, 6, z), solid(physics.ModelBox(grass4))...)
		}
	}
	_, rock := load("crumbling_rock_platform.obj")
	prop("crumbling_rock_platform.obj", at(14.5, 0.3, -4),
		illusion.C(physics.Kinematic), illusion.C(physics.ModelBox(rock)),
		illusion.C(Mover{From: rl.Vector3{X: 14.5, Y: 0.3, Z: -4}, To: rl.Vector3{X: 14.5, Y: 6, Z: -4}, Period: 8}),
	)
	_, spring := load("spring.obj")
	prop("spring.obj", at(9.5, 0, -6), illusion.C(Spring{}),
		illusion.C(physics.Static), illusion.C(physics.ModelBox(spring)), illusion.C(physics.Sensor{}))
	prop("sign_arrow.obj", at(7.5, 0, -4).WithRotation(rl.QuaternionFromAxisAngle(transform.Up, math.Pi)))

	// The treasure: a solid chest with a slightly larger trigger around it.
	_, chest := load("treasure_chest.obj")
	chestAt := at(22, 6, -5).WithRotation(rl.QuaternionFromAxisAngle(transform.Up, -math.Pi/2))
	prop("treasure_chest.obj", chestAt, solid(physics.ModelBox(chest))...)
	cmd.Spawn(illusion.C(Treasure{}), illusion.C(chestAt),
		illusion.C(physics.Static), illusion.C(physics.Sensor{}), illusion.C(physics.Cuboid(2, 1.6, 2).WithOffset(rl.Vector3{Y: 0.6})))
	prop("flag.obj", at(24, 6, -7.5))
	prop("trophy.obj", at(20, 6, -7.5))

	// Crate stacks to knock over.
	_, crate := load("crate.obj")
	crateBox := physics.ModelBox(crate)
	stack := func(base rl.Vector3, rows int) {
		for row := 0; row < rows; row++ {
			for i := 0; i < rows-row; i++ {
				x := base.X + float32(i)*1.02 + float32(row)*0.51
				prop("crate.obj", at(x, base.Y+float32(row)*1.01, base.Z),
					illusion.C(physics.Dynamic), illusion.C(crateBox), illusion.C(physics.Mass(4)),
					illusion.C(physics.Material{Friction: 0.6}))
			}
		}
	}
	stack(rl.Vector3{X: 2, Z: -3}, 4)
	stack(rl.Vector3{X: -8, Z: -7}, 3)

	// A spiked log rolling in place next to the second stack.
	_, log := load("rotating_log.obj")
	prop("rotating_log.obj", at(-5, 1.45, -3),
		illusion.C(physics.Kinematic), illusion.C(physics.ModelConvexHull(log)), illusion.C(Roller{Speed: 1.5}))

	// Coins: a sensor at the root, a spinning model on top.
	coins := []rl.Vector3{
		{X: 0, Y: 1, Z: 0}, {X: -4, Y: 1, Z: 4}, {X: 4, Y: 1, Z: 4}, {X: -9, Y: 1, Z: 9},
		{X: 9, Y: 1, Z: 9}, {X: 2.8, Y: 5, Z: -3}, {X: 14.5, Y: 8, Z: -4}, {X: 20, Y: 7, Z: -2}, {X: 24, Y: 7, Z: -2},
	}
	coinModel, _ := load("coin.obj")
	for _, p := range coins {
		cmd.Spawn(illusion.C(Coin{}), illusion.C(transform.FromTranslation(p)),
			illusion.C(physics.Static), illusion.C(physics.Sensor{}), illusion.C(physics.Sphere(0.5))).
			WithChild(
				illusion.C(render.Model3d{Model: coinModel}), illusion.C(kitMaterial),
				illusion.C(transform.Identity().WithScale(1.4)), illusion.C(Spinner{Speed: 3}),
			)
	}
	cmd.Queue(func(w *ecs.World) { ecs.GetResource[Game](w).TotalCoins = len(coins) })

	// Scenery with simple colliders.
	for _, t := range []struct {
		file string
		x, z float32
	}{{"tree_1.obj", -10, -10}, {"tree_2.obj", -10, 9}, {"tree_1.obj", 10, 10}, {"tree_2.obj", 0, -10.5}} {
		prop(t.file, at(t.x, 0, t.z), illusion.C(physics.Static),
			illusion.C(physics.Cylinder(0.35, 3).WithOffset(rl.Vector3{Y: 1.5})))
	}
	_, fence := load("fence_4.obj")
	for x := float32(-10.3); x <= 10.3; x += 3.44 {
		prop("fence_4.obj", at(x, 0, 11.6), solid(physics.ModelBox(fence))...)
	}
	for _, d := range []struct {
		file    string
		x, z, s float32
	}{
		{"flower.obj", -3, 3, 1.5}, {"flower.obj", 5, 7, 1.5}, {"flower.obj", -7, -2, 1.5},
		{"grass_blades_1.obj", -1, 8, 1}, {"grass_blades_2.obj", 6, -8, 1}, {"grass_blades_3.obj", -9, 3, 1},
		{"mushroom.obj", 8, 0, 1.4}, {"mushroom.obj", -4, -9, 1.2}, {"plant.obj", -11, -2, 1},
		{"lilypad.obj", 14, 8, 1}, {"lilypad.obj", -14, -6, 1},
		{"rock_1.obj", -6, 2, 2.5}, {"rock_2.obj", 7, 3, 3}, {"rock_3.obj", -2, 9, 2.5}, {"rock_1.obj", 11, -10, 2},
	} {
		y := float32(0)
		if d.file == "lilypad.obj" {
			y = -0.55
		}
		prop(d.file, at(d.x, y, d.z).WithScale(d.s))
	}

	// Clouds and balloons drift around.
	for _, c := range []struct {
		file    string
		p       rl.Vector3
		speed   float32
		balloon bool
	}{
		{"cloud_1.obj", rl.Vector3{X: -8, Y: 13, Z: -14}, 0.15, false},
		{"cloud_2.obj", rl.Vector3{X: 10, Y: 15, Z: -18}, 0.1, false},
		{"cloud_3.obj", rl.Vector3{X: -18, Y: 11, Z: 2}, 0.12, false},
		{"balloon_red.obj", rl.Vector3{X: -3, Y: 4, Z: -1}, 0.8, true},
		{"balloon_blue.obj", rl.Vector3{X: 5, Y: 5, Z: 2}, 0.7, true},
		{"balloon_yellow.obj", rl.Vector3{X: 21, Y: 9, Z: -4}, 0.9, true},
	} {
		amount := rl.Vector3{X: 6}
		scale := float32(2)
		if c.balloon {
			amount, scale = rl.Vector3{Y: 0.4}, 1.5
		}
		prop(c.file, transform.FromTranslation(c.p).WithScale(scale), illusion.C(Drift{Origin: c.p, Amount: amount, Speed: c.speed}))
	}

	// The bee: a character controller at the root, the model as a child.
	beeModel, _ := load("bee.obj")
	cmd.Spawn(
		illusion.C(Player{}),
		illusion.C(physics.CharacterController{Radius: 0.4, Height: 1.2, StepHeight: 0.35}),
		illusion.C(transform.FromTranslation(spawnPoint)),
	).WithChild(
		illusion.C(Body{}), illusion.C(render.Model3d{Model: beeModel}), illusion.C(kitMaterial),
		illusion.C(transform.FromXYZ(0, -0.25, 0)),
	)
}

// walk turns WASD into the controller's Walk velocity and jumps with Space.
func walk(
	players *illusion.Query1Where[physics.CharacterController, illusion.With[Player]],
	bodies *illusion.Query1Where[transform.Transform, illusion.With[Body]],
	keys *illusion.Res[input.Keys],
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
	if rl.Vector3LengthSqr(dir) > 0 {
		dir = rl.Vector3Normalize(dir)
	}
	players.Each(func(_ ecs.Entity, cc *physics.CharacterController) {
		cc.Walk = rl.Vector3Scale(dir, walkSpeed)
		if k.JustPressed(rl.KeySpace) && cc.Grounded {
			cc.Velocity.Y = jumpSpeed
		}
	})
	if dir != (rl.Vector3{}) {
		// The bee model faces Forward (-Z); yaw it toward the walk direction.
		face := rl.QuaternionFromAxisAngle(transform.Up, float32(math.Atan2(float64(-dir.X), float64(-dir.Z))))
		bodies.Each(func(_ ecs.Entity, tr *transform.Transform) {
			tr.Rotation = rl.QuaternionSlerp(tr.Rotation, face, 0.2)
		})
	}
}

func follow(
	cameras *illusion.Query2[FollowCamera, transform.Transform],
	players *illusion.Query1Where[transform.Transform, illusion.With[Player]],
) {
	_, player, ok := players.Single()
	if !ok {
		return
	}
	target := rl.Vector3Add(player.Translation, rl.Vector3{Y: 0.8})
	cameras.Each(func(_ ecs.Entity, f *FollowCamera, tr *transform.Transform) {
		tr.Translation = rl.Vector3Lerp(tr.Translation, rl.Vector3Add(player.Translation, f.Offset), 0.08)
		tr.LookAt(target, transform.Up)
	})
}

func throwBomb(
	keys *illusion.Res[input.Keys],
	buttons *illusion.Res[input.MouseButtons],
	bees *illusion.Query2[transform.GlobalTransform, Body],
	kit *illusion.Res[Kit],
	cmd *illusion.Commands,
) {
	if !keys.Get().JustPressed(rl.KeyE) && !buttons.Get().JustPressed(rl.MouseButtonLeft) {
		return
	}
	_, g, _, ok := bees.Single()
	if !ok {
		return
	}
	k := kit.Get()
	forward := g.Forward()
	start := rl.Vector3Add(g.Translation(), rl.Vector3Add(rl.Vector3Scale(forward, 1), rl.Vector3{Y: 0.2}))
	velocity := rl.Vector3Add(rl.Vector3Scale(forward, bombSpeed), rl.Vector3{Y: 4})
	// Colliders ignore Transform.Scale, so the bomb is spawned at full size.
	cmd.Spawn(
		illusion.C(Bomb{Fuse: 2}),
		illusion.C(render.Model3d{Model: k.Bomb}), illusion.C(k.Material),
		illusion.C(transform.FromTranslation(start)),
		illusion.C(physics.Dynamic), illusion.C(k.BombHull), illusion.C(physics.Mass(1)),
		illusion.C(physics.Material{Friction: 0.8, Restitution: 0.3}),
		illusion.C(physics.Velocity{Linear: velocity, Angular: rl.Vector3{X: 4}}),
	)
}

// Explosion is sent when a bomb goes off.
type Explosion struct{ At rl.Vector3 }

// fuses counts bombs down and blows them up.
func fuses(
	bombs *illusion.Query2[Bomb, transform.Transform],
	t *illusion.Res[illusion.Time],
	kit *illusion.Res[Kit],
	explosions *illusion.EventWriter[Explosion],
	cmd *illusion.Commands,
) {
	dt := t.Get().DeltaSecs()
	k := kit.Get()
	bombs.Each(func(e ecs.Entity, b *Bomb, tr *transform.Transform) {
		b.Fuse -= dt
		if b.Fuse > 0 {
			return
		}
		cmd.Despawn(e)
		center := rl.Vector3Add(tr.Translation, rl.Vector3{Y: 0.5})
		explosions.Send(Explosion{At: center})
		cmd.Spawn(illusion.C(Blast{}), illusion.C(render.Mesh3d{Mesh: k.BlastMesh}), illusion.C(k.BlastColor),
			illusion.C(transform.FromTranslation(center).WithScale(0.1)))
	})
}

// blastForces pushes every dynamic body and the player away from explosions.
func blastForces(
	explosions *illusion.EventReader[Explosion],
	bodies *illusion.Query2[physics.RigidBody, transform.Transform],
	players *illusion.Query2Where[physics.CharacterController, transform.Transform, illusion.With[Player]],
	phys *physics.Physics,
) {
	push := func(center, at rl.Vector3) (rl.Vector3, bool) {
		d := rl.Vector3Subtract(at, center)
		dist := rl.Vector3Length(d)
		if dist > blastRadius {
			return rl.Vector3{}, false
		}
		dir := rl.Vector3Normalize(rl.Vector3Add(d, rl.Vector3{Y: 0.6}))
		return rl.Vector3Scale(dir, blastPower*(1-dist/blastRadius)), true
	}
	for ex := range explosions.Read() {
		bodies.Each(func(e ecs.Entity, rb *physics.RigidBody, tr *transform.Transform) {
			if *rb != physics.Dynamic {
				return
			}
			if impulse, ok := push(ex.At, tr.Translation); ok {
				phys.AddImpulse(e, impulse)
			}
		})
		players.Each(func(_ ecs.Entity, cc *physics.CharacterController, tr *transform.Transform) {
			if kick, ok := push(ex.At, tr.Translation); ok {
				cc.Velocity = rl.Vector3Add(cc.Velocity, rl.Vector3Scale(kick, 0.25))
			}
		})
	}
}

// blasts grows each explosion flash and removes it.
func blasts(q *illusion.Query2[Blast, transform.Transform], t *illusion.Res[illusion.Time], cmd *illusion.Commands) {
	dt := t.Get().DeltaSecs()
	q.Each(func(e ecs.Entity, b *Blast, tr *transform.Transform) {
		b.Age += dt
		tr.Scale = rl.Vector3Scale(rl.Vector3One(), 0.1+b.Age/0.3*blastRadius*0.5)
		if b.Age >= 0.3 {
			cmd.Despawn(e)
		}
	})
}

// tags answers "what kind of thing is this entity?" for collision handling.
type tags struct {
	coins    illusion.Query0Where[illusion.With[Coin]]
	springs  illusion.Query0Where[illusion.With[Spring]]
	treasure illusion.Query0Where[illusion.With[Treasure]]
}

func (t *tags) InitParam(w *ecs.World) {
	t.coins.InitParam(w)
	t.springs.InitParam(w)
	t.treasure.InitParam(w)
}

// touches reacts to the player bumping into coins, springs and the treasure.
func touches(
	started *illusion.EventReader[physics.CollisionStarted],
	players *illusion.Query1Where[physics.CharacterController, illusion.With[Player]],
	is *tags,
	game *illusion.Res[Game],
	cmd *illusion.Commands,
) {
	player, cc, ok := players.Single()
	if !ok {
		started.Clear()
		return
	}
	g := game.Get()
	taken := map[ecs.Entity]bool{}
	for ev := range started.Read() {
		other, ok := ev.Involves(player)
		if !ok {
			continue
		}
		switch {
		case is.coins.Contains(other) && !taken[other]:
			taken[other] = true
			g.Coins++
			cmd.Despawn(other)
		case is.springs.Contains(other):
			cc.Velocity.Y = springSpeed
		case is.treasure.Contains(other):
			g.Won = true
		}
	}
}

func respawn(
	players *illusion.Query2Where[physics.CharacterController, transform.Transform, illusion.With[Player]],
	keys *illusion.Res[input.Keys],
) {
	r := keys.Get().JustPressed(rl.KeyR)
	players.Each(func(_ ecs.Entity, cc *physics.CharacterController, tr *transform.Transform) {
		if r || tr.Translation.Y < -12 {
			tr.Translation = spawnPoint
			cc.Velocity = rl.Vector3{}
		}
	})
}

func moveMovers(q *illusion.Query2[Mover, transform.Transform], t *illusion.Res[illusion.Time]) {
	elapsed := float64(t.Get().ElapsedSecs())
	q.Each(func(_ ecs.Entity, m *Mover, tr *transform.Transform) {
		// Ease back and forth: 0 at From, 1 at To.
		phase := float32(0.5 - 0.5*math.Cos(2*math.Pi*elapsed/float64(m.Period)))
		tr.Translation = rl.Vector3Lerp(m.From, m.To, phase)
	})
}

func rollRollers(q *illusion.Query2[Roller, transform.Transform], t *illusion.Res[illusion.Time]) {
	dt := t.Get().DeltaSecs()
	q.Each(func(_ ecs.Entity, r *Roller, tr *transform.Transform) {
		tr.RotateX(r.Speed * dt)
	})
}

func spin(q *illusion.Query2[Spinner, transform.Transform], t *illusion.Res[illusion.Time]) {
	dt := t.Get().DeltaSecs()
	q.Each(func(_ ecs.Entity, s *Spinner, tr *transform.Transform) {
		tr.RotateY(s.Speed * dt)
	})
}

func drift(q *illusion.Query2[Drift, transform.Transform], t *illusion.Res[illusion.Time]) {
	elapsed := float64(t.Get().ElapsedSecs())
	q.Each(func(_ ecs.Entity, d *Drift, tr *transform.Transform) {
		tr.Translation = rl.Vector3Add(d.Origin, rl.Vector3Scale(d.Amount, float32(math.Sin(elapsed*float64(d.Speed)))))
	})
}

func hud(game *illusion.Res[Game], win *illusion.Res[window.Window]) {
	g, w := game.Get(), win.Get()
	rl.DrawText(fmt.Sprintf("coins %d/%d", g.Coins, g.TotalCoins), 16, 16, 28, rl.Gold)
	rl.DrawText("WASD move   space jump   E/click bomb   R respawn", 16, 50, 20, rl.White)
	rl.DrawFPS(int32(w.Width)-90, 12)
	if g.Won {
		msg := fmt.Sprintf("You found the treasure with %d of %d coins!", g.Coins, g.TotalCoins)
		width := rl.MeasureText(msg, 36)
		rl.DrawRectangle(0, int32(w.Height)/2-40, int32(w.Width), 80, rl.Fade(rl.Black, 0.5))
		rl.DrawText(msg, int32(w.Width)/2-width/2, int32(w.Height)/2-18, 36, rl.Gold)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
