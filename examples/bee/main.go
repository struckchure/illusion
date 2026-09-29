// A bee in a physics garden: a character controller walks it around, crates
// and balls get shoved and knocked over, and bumping into flowers pollinates
// them (read from collision events). The bee is a parent/child hierarchy with
// a model loaded through the asset loader.
//
// WASD walks, Space jumps (hold it in the air to hover), F stings whatever is
// in front of the bee, P pops the pollen, F3 shows stats, F4 shows physics
// colliders, Escape quits.
package main

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/audio"
	"github.com/struckchure/illusion/engine"
	"github.com/struckchure/illusion/input"
	"github.com/struckchure/illusion/physics"
	"github.com/struckchure/illusion/render"
	"github.com/struckchure/illusion/transform"
	"github.com/struckchure/illusion/window"
)

const (
	walkSpeed  = 5
	jumpSpeed  = 6
	hoverSpeed = -1 // slowest fall while hovering
	turnSpeed  = 10
	bodyHeight = -0.15 // model offset from the capsule's center
	stingRange = 3
)

// Player is the pivot the bee's parts hang from; it carries the character
// controller.
type Player struct{}

// Body is the bee model; it bobs and turns to face where it's going.
type Body struct{}

// Pollen marks the orbiting pollen group.
type Pollen struct{}

// Flower marks a flower stem; Pollinated flips when the bee bumps into it.
type Flower struct{ Pollinated bool }

// Spin rotates an entity around its parent's Y axis.
type Spin struct{ Speed float32 }

// FollowCamera trails the player.
type FollowCamera struct{ Offset rl.Vector3 }

// Garden tracks progress.
type Garden struct{ Pollinated, Flowers int }

// Sfx holds the synthesized sound effects.
type Sfx struct{ Sting, Pollinate asset.Handle[audio.Sound] }

func main() {
	illusion.New().
		AddPlugins(
			engine.DefaultPlugins(engine.Config{
				Window:    window.Config{Title: "Illusion: bee", MSAA: true},
				AssetRoot: "examples/assets",
			}),
			physics.Plugin{},
			physics.DebugPlugin{Hidden: true}, // F4 shows collider outlines
		).
		InsertResource(
			illusion.R(&render.ClearColor{Color: rl.NewColor(150, 200, 240, 255)}),
			illusion.R(&Garden{}),
		).
		AddSystems(illusion.Startup, illusion.Fn6(setup), illusion.Fn2(loadSounds)).
		AddSystems(illusion.FixedUpdate, illusion.Fn2(walk)).
		AddSystems(illusion.Update,
			illusion.Fn2(follow),
			illusion.Fn3(faceAndBob),
			illusion.Fn2(spin),
			illusion.Fn3(popPollen),
			illusion.Fn7(sting),
			illusion.Fn7(pollinate),
		).
		AddSystems(illusion.Render, illusion.Fn2(hud).InSet(render.Draw2D)).
		Run()
}

func setup(
	cmd *illusion.Commands,
	models *asset.Loader[render.Model],
	textures *asset.Loader[render.Texture],
	meshes *illusion.Res[asset.Assets[render.Mesh]],
	materials *illusion.Res[asset.Assets[render.StandardMaterial]],
	garden *illusion.Res[Garden],
) {
	mesh, mat := meshes.Get(), materials.Get()
	color := func(c rl.Color) render.MeshMaterial3d {
		return render.MeshMaterial3d{Material: mat.Add(render.StandardMaterial{BaseColor: c})}
	}

	cmd.Spawn(
		illusion.C(render.Camera3d{Fovy: 50}),
		illusion.C(FollowCamera{Offset: rl.Vector3{Y: 4, Z: 9}}),
		illusion.C(transform.FromXYZ(0, 5, 10).LookingAt(rl.Vector3{}, transform.Up)),
	)
	cmd.Spawn(
		illusion.C(render.DirectionalLight{Color: rl.NewColor(255, 244, 214, 255)}),
		illusion.C(transform.Identity().LookingAt(rl.Vector3{X: -1, Y: -2, Z: -1.5}, transform.Up)),
	)

	// Ground: a flat plane to look at, a thick static box to stand on.
	cmd.Spawn(
		illusion.C(render.Mesh3d{Mesh: mesh.Add(render.Plane(60, 60))}),
		illusion.C(color(rl.NewColor(92, 140, 76, 255))),
		illusion.C(transform.Identity()),
		illusion.C(physics.Static),
		illusion.C(physics.Cuboid(60, 1, 60).WithOffset(rl.Vector3{Y: -0.5})),
	)

	// Flowers: a static stem with a petal child and a center grandchild.
	stem, petal, center := mesh.Add(render.Cylinder(0.08, 1.5)), mesh.Add(render.Torus(0.35, 0.12)), mesh.Add(render.Sphere(0.25))
	colors := []rl.Color{rl.Pink, rl.Purple, rl.Orange, rl.SkyBlue, rl.Red, rl.Gold}
	garden.Get().Flowers = len(colors)
	for i, c := range colors {
		angle := float64(i) * 2 * math.Pi / float64(len(colors))
		x, z := float32(math.Cos(angle))*8, float32(math.Sin(angle))*8
		cmd.Spawn(
			illusion.C(Flower{}),
			illusion.C(render.Mesh3d{Mesh: stem}),
			illusion.C(color(rl.DarkGreen)),
			illusion.C(transform.FromXYZ(x, 0, z)),
			illusion.C(physics.Static),
			// render.Cylinder sits on its base; the collider is centered, so lift it.
			illusion.C(physics.Cylinder(0.25, 2).WithOffset(rl.Vector3{Y: 1})),
		).WithChildren(func(c1 *illusion.ChildBuilder) {
			c1.Spawn(
				illusion.C(render.Mesh3d{Mesh: petal}),
				illusion.C(color(c)),
				illusion.C(transform.FromXYZ(0, 1.6, 0).WithRotation(rl.QuaternionFromAxisAngle(transform.Right, transform.Deg(70)))),
			).WithChild(
				illusion.C(render.Mesh3d{Mesh: center}),
				illusion.C(color(rl.Yellow)),
				illusion.C(transform.Identity()),
			)
		})
	}

	// A pyramid of crates and a few bouncy balls to knock around.
	crate, crateColor := mesh.Add(render.Cuboid(0.8, 0.8, 0.8)), color(rl.NewColor(181, 128, 72, 255))
	for row := range 3 {
		for i := range 3 - row {
			x := -4 + float32(i)*0.82 + float32(row)*0.41
			cmd.Spawn(
				illusion.C(render.Mesh3d{Mesh: crate}),
				illusion.C(crateColor),
				illusion.C(transform.FromXYZ(x, 0.4+float32(row)*0.8, -3)),
				illusion.C(physics.Dynamic),
				illusion.C(physics.Cuboid(0.8, 0.8, 0.8)),
				illusion.C(physics.Material{Friction: 0.7}),
				// Mass defaults to the shape's volume at water density: a
				// 512 kg crate. Make it light enough for a bee to shove.
				illusion.C(physics.Mass(4)),
			)
		}
	}
	ball := mesh.Add(render.Sphere(0.4))
	for i, c := range []rl.Color{rl.Red, rl.Blue, rl.Lime} {
		cmd.Spawn(
			illusion.C(render.Mesh3d{Mesh: ball}),
			illusion.C(color(c)),
			illusion.C(transform.FromXYZ(3+float32(i)*1.2, 3+float32(i), -2)),
			illusion.C(physics.Dynamic),
			illusion.C(physics.Sphere(0.4)),
			illusion.C(physics.Material{Friction: 0.4, Restitution: 0.7}),
			illusion.C(physics.Mass(1)),
		)
	}

	// The bee: a character-controlled pivot with the body, a halo and orbiting
	// pollen underneath.
	beeMaterial := render.MeshMaterial3d{Material: mat.Add(render.StandardMaterial{
		BaseColor: rl.White,
		Texture:   textures.MustLoad("palette.png"),
	})}
	cmd.Spawn(
		illusion.C(Player{}),
		illusion.C(physics.CharacterController{Radius: 0.45, Height: 1.3, StepHeight: 0.25}),
		illusion.C(transform.FromXYZ(0, 2, 2)),
	).WithChildren(func(c *illusion.ChildBuilder) {
		c.Spawn(
			illusion.C(Body{}),
			illusion.C(render.Model3d{Model: models.MustLoad("bee.obj")}),
			illusion.C(beeMaterial),
			illusion.C(transform.FromXYZ(0, bodyHeight, 0).WithScale(1.5)),
		).WithChild(
			illusion.C(render.Mesh3d{Mesh: mesh.Add(render.Torus(0.18, 0.03))}),
			illusion.C(render.MeshMaterial3d{Material: mat.Add(render.StandardMaterial{BaseColor: rl.Gold, Unlit: true})}),
			illusion.C(transform.FromXYZ(0, 0.9, 0).WithRotation(rl.QuaternionFromAxisAngle(transform.Right, transform.Deg(90)))),
		)
		c.Spawn(illusion.C(Pollen{}), illusion.C(Spin{Speed: 2}), illusion.C(transform.Identity())).
			WithChildren(func(c *illusion.ChildBuilder) {
				grain := mesh.Add(render.Sphere(0.12))
				for _, x := range []float32{-1.2, 1.2} {
					c.Spawn(
						illusion.C(render.Mesh3d{Mesh: grain}),
						illusion.C(color(rl.Yellow)),
						illusion.C(transform.FromXYZ(x, 0.2, 0)),
					)
				}
			})
	})
}

// loadSounds synthesizes the effects; there are no audio files in the repo.
func loadSounds(cmd *illusion.Commands, sounds *illusion.Res[asset.Assets[audio.Sound]]) {
	s := sounds.Get()
	cmd.InsertResource(illusion.R(&Sfx{
		Sting:     s.Add(audio.SoundFromSamples(audio.Tone(220, 0.12), audio.SampleRate)),
		Pollinate: s.Add(audio.SoundFromSamples(audio.Tone(880, 0.25), audio.SampleRate)),
	}))
}

// walk runs in FixedUpdate, feeding input to the character controller.
func walk(q *illusion.Query1Where[physics.CharacterController, illusion.With[Player]], keys *illusion.Res[input.Keys]) {
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
		dir = rl.Vector3Scale(rl.Vector3Normalize(dir), walkSpeed)
	}
	q.Each(func(_ ecs.Entity, c *physics.CharacterController) {
		c.Walk = dir
		switch {
		case c.Grounded && k.Pressed(rl.KeySpace):
			c.Velocity.Y = jumpSpeed
		case !c.Grounded && k.Pressed(rl.KeySpace) && c.Velocity.Y < hoverSpeed:
			c.Velocity.Y = hoverSpeed
		}
	})
}

// faceAndBob turns the body toward the walk direction and bobs it.
func faceAndBob(
	bodies *illusion.Query1Where[transform.Transform, illusion.With[Body]],
	players *illusion.Query1Where[physics.CharacterController, illusion.With[Player]],
	t *illusion.Res[illusion.Time],
) {
	_, player, ok := players.Single()
	if !ok {
		return
	}
	time := t.Get()
	walk := player.Walk
	bodies.Each(func(_ ecs.Entity, tr *transform.Transform) {
		if walk.X != 0 || walk.Z != 0 {
			// The model faces Forward (-Z); yaw it toward the walk direction.
			target := rl.QuaternionFromAxisAngle(transform.Up, float32(math.Atan2(float64(-walk.X), float64(-walk.Z))))
			tr.Rotation = rl.QuaternionSlerp(tr.Rotation, target, min(1, turnSpeed*time.DeltaSecs()))
		}
		tr.Translation.Y = bodyHeight + 0.1*float32(math.Sin(float64(time.ElapsedSecs())*4))
	})
}

func spin(q *illusion.Query2[Spin, transform.Transform], t *illusion.Res[illusion.Time]) {
	dt := t.Get().DeltaSecs()
	q.Each(func(_ ecs.Entity, s *Spin, tr *transform.Transform) {
		tr.RotateY(s.Speed * dt)
	})
}

func popPollen(keys *illusion.Res[input.Keys], pollen *illusion.Query0Where[illusion.With[Pollen]], cmd *illusion.Commands) {
	if keys.Get().JustPressed(rl.KeyP) {
		if e, ok := pollen.Single(); ok {
			cmd.Despawn(e) // takes both grains with it
		}
	}
}

// sting knocks back whatever dynamic body is right in front of the bee.
func sting(
	keys *illusion.Res[input.Keys],
	players *illusion.Query1Where[transform.Transform, illusion.With[Player]],
	bodies *illusion.Query1Where[transform.GlobalTransform, illusion.With[Body]],
	rigid *illusion.Query1[physics.RigidBody],
	phys *physics.Physics,
	sfx *audio.Audio,
	sounds *illusion.Res[Sfx],
) {
	if !keys.Get().JustPressed(rl.KeyF) {
		return
	}
	sfx.Play(sounds.Get().Sting)
	player, pt, ok := players.Single()
	_, body, ok2 := bodies.Single()
	if !ok || !ok2 {
		return
	}
	forward := body.Forward()
	hit, ok := phys.CastRayExcluding(pt.Translation, forward, stingRange, player)
	if !ok {
		return
	}
	if rb, ok := rigid.Get(hit.Entity); ok && *rb == physics.Dynamic {
		phys.AddImpulse(hit.Entity, rl.Vector3Add(rl.Vector3Scale(forward, 40), rl.Vector3{Y: 12}))
	}
}

// pollinate marks flowers the bee bumps into and sets them spinning.
func pollinate(
	collisions *illusion.EventReader[physics.CollisionStarted],
	players *illusion.Query0Where[illusion.With[Player]],
	flowers *illusion.Query1[Flower],
	garden *illusion.Res[Garden],
	cmd *illusion.Commands,
	sfx *audio.Audio,
	sounds *illusion.Res[Sfx],
) {
	player, ok := players.Single()
	for ev := range collisions.Read() {
		if !ok {
			continue
		}
		other, involved := ev.Involves(player)
		if !involved {
			continue
		}
		if f, isFlower := flowers.Get(other); isFlower && !f.Pollinated {
			f.Pollinated = true
			garden.Get().Pollinated++
			cmd.Entity(other).Insert(illusion.C(Spin{Speed: 3}))
			sfx.PlayWith(sounds.Get().Pollinate, audio.Playback{Pitch: 1 + 0.1*float32(garden.Get().Pollinated)})
		}
	}
}

// follow trails the player, whose Transform is its world position (it's a
// root entity).
func follow(
	cameras *illusion.Query2[FollowCamera, transform.Transform],
	players *illusion.Query1Where[transform.Transform, illusion.With[Player]],
) {
	_, player, ok := players.Single()
	if !ok {
		return
	}
	target := player.Translation
	cameras.Each(func(_ ecs.Entity, f *FollowCamera, tr *transform.Transform) {
		tr.Translation = rl.Vector3Lerp(tr.Translation, rl.Vector3Add(target, f.Offset), 0.1)
		tr.LookAt(target, transform.Up)
	})
}

func hud(garden *illusion.Res[Garden], players *illusion.Query1Where[physics.CharacterController, illusion.With[Player]]) {
	rl.DrawFPS(10, 10)
	rl.DrawText("WASD walk   space jump/hover   F sting   P pop pollen   F3 stats   F4 colliders", 10, 36, 20, rl.RayWhite)
	g := garden.Get()
	status := fmt.Sprintf("flowers pollinated: %d/%d", g.Pollinated, g.Flowers)
	if _, c, ok := players.Single(); ok && !c.Grounded {
		status += "   (airborne)"
	}
	rl.DrawText(status, 10, 60, 20, rl.RayWhite)
}
