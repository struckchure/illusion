package physics

import (
	"math"
	"testing"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/diag"
	"github.com/struckchure/illusion/transform"
)

const frame = time.Second / 60

type tag string

// newApp returns an app with physics and a 100x1x100 static floor whose top
// is at y=0.
func newApp(t *testing.T, setup func(cmd *illusion.Commands)) *illusion.App {
	t.Helper()
	app := illusion.New().AddPlugins(transform.Plugin{}, Plugin{})
	app.AddSystems(illusion.Startup, illusion.Fn1(func(cmd *illusion.Commands) {
		cmd.Spawn(
			illusion.C(tag("floor")),
			illusion.C(Static),
			illusion.C(Cuboid(100, 1, 100)),
			illusion.C(transform.FromXYZ(0, -0.5, 0)),
		)
		setup(cmd)
	}))
	t.Cleanup(app.Cleanup)
	return app
}

func run(app *illusion.App, seconds float64) {
	for range int(seconds * 60) {
		app.Tick(frame)
	}
}

func find(app *illusion.App, name tag) ecs.Entity {
	var found ecs.Entity
	query := ecs.NewFilter1[tag](app.World).Query()
	for query.Next() {
		if *query.Get() == name {
			found = query.Entity()
		}
	}
	return found
}

func translation(app *illusion.App, e ecs.Entity) rl.Vector3 {
	return ecs.NewMap[transform.Transform](app.World).Get(e).Translation
}

func TestBodiesFallCollideAndDespawn(t *testing.T) {
	var started []CollisionStarted
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(
			illusion.C(tag("crate")),
			illusion.C(Dynamic),
			illusion.C(Cuboid(1, 1, 1)),
			illusion.C(transform.FromXYZ(0, 4, 0)),
			illusion.C(Velocity{}),
		)
	})
	app.AddSystems(illusion.Update, illusion.Fn1(func(r *illusion.EventReader[CollisionStarted]) {
		for ev := range r.Read() {
			started = append(started, ev)
		}
	}))
	run(app, 3)

	crate, floor := find(app, "crate"), find(app, "floor")
	if y := translation(app, crate).Y; math.Abs(float64(y-0.5)) > 0.05 {
		t.Fatalf("crate should rest at y=0.5, got %v", y)
	}
	hit := false
	for _, ev := range started {
		if other, ok := ev.Involves(crate); ok && other == floor {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("expected a crate/floor collision, got %+v", started)
	}

	// Despawning removes the body from the simulation.
	app.AddSystems(illusion.Update, illusion.Fn1(func(cmd *illusion.Commands) { cmd.Despawn(crate) }).RunIf(illusion.Once()))
	run(app, 0.1)
	var p Physics
	p.InitParam(app.World)
	if hit, ok := p.CastRay(rl.Vector3{X: -5, Y: 0.5}, rl.Vector3{X: 1}, 10); ok && hit.Entity == crate {
		t.Fatal("despawned crate is still in the physics world")
	}
	if n := len(ecs.GetResource[world](app.World).entities); n != 1 {
		t.Fatalf("only the floor should remain mapped, got %d bodies", n)
	}
}

func TestVelocityComponentAndImpulse(t *testing.T) {
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(
			illusion.C(tag("puck")),
			illusion.C(Dynamic),
			illusion.C(Sphere(0.5)),
			illusion.C(GravityScale(0)),
			illusion.C(Damping{}),
			illusion.C(transform.FromXYZ(0, 5, 0)),
			illusion.C(Velocity{Linear: rl.Vector3{X: 3}}),
		)
	})
	run(app, 1)
	puck := find(app, "puck")
	if x := translation(app, puck).X; math.Abs(float64(x-3)) > 0.15 {
		t.Fatalf("puck should travel ~3m in 1s, got x=%v", x)
	}

	// Writing Velocity changes the body's velocity.
	ecs.NewMap[Velocity](app.World).Get(puck).Linear = rl.Vector3{Z: -2}
	run(app, 0.5)
	if p := translation(app, puck); math.Abs(float64(p.Z+1)) > 0.1 || math.Abs(float64(p.X-3)) > 0.15 {
		t.Fatalf("puck should now move along -Z, at %v", p)
	}

	app.AddSystems(illusion.FixedUpdate, illusion.Fn1(func(p *Physics) {
		p.AddImpulse(puck, rl.Vector3{Y: 10})
	}).RunIf(illusion.Once()))
	run(app, 0.1)
	if v := ecs.NewMap[Velocity](app.World).Get(puck).Linear; v.Y <= 0 {
		t.Fatalf("impulse should push the puck up, velocity %v", v)
	}
}

func TestTeleportAndKinematicPush(t *testing.T) {
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(tag("ball")), illusion.C(Dynamic), illusion.C(Sphere(0.5)),
			illusion.C(transform.FromXYZ(0, 0.5, 0)))
		cmd.Spawn(illusion.C(tag("pusher")), illusion.C(Kinematic), illusion.C(Cuboid(1, 2, 3)),
			illusion.C(transform.FromXYZ(-3, 1, 0)))
	})
	run(app, 0.5)
	ball, pusher := find(app, "ball"), find(app, "pusher")

	// Sweep the kinematic wall through the ball.
	app.AddSystems(illusion.FixedUpdate, illusion.Fn2(func(q *illusion.Query2[RigidBody, transform.Transform], t *illusion.Res[illusion.Time]) {
		q.Each(func(_ ecs.Entity, rb *RigidBody, tr *transform.Transform) {
			if *rb == Kinematic {
				tr.Translation.X += 4 * t.Get().DeltaSecs()
			}
		})
	}))
	run(app, 1.5)
	if x, px := translation(app, ball).X, translation(app, pusher).X; x < px+0.9 {
		t.Fatalf("the kinematic wall should push the ball ahead of it: ball %v wall %v", x, px)
	}

	// Moving a dynamic body's Transform teleports it (keeping its velocity).
	ecs.NewMap[transform.Transform](app.World).Get(ball).Translation = rl.Vector3{X: 20, Y: 3, Z: 20}
	run(app, 0.05)
	if p := translation(app, ball); math.Abs(float64(p.X-20)) > 0.5 || math.Abs(float64(p.Z-20)) > 0.5 {
		t.Fatalf("ball should have teleported, at %v", p)
	}
}

func TestSensorsAndRaycast(t *testing.T) {
	var sensed []ecs.Entity
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(tag("zone")), illusion.C(Static), illusion.C(Sensor{}), illusion.C(Cuboid(4, 1, 4)),
			illusion.C(transform.FromXYZ(0, 2, 0)))
		cmd.Spawn(illusion.C(tag("rock")), illusion.C(Dynamic), illusion.C(Sphere(0.3)),
			illusion.C(transform.FromXYZ(0, 6, 0)))
	})
	app.AddSystems(illusion.Update, illusion.Fn1(func(r *illusion.EventReader[CollisionStarted]) {
		for ev := range r.Read() {
			if other, ok := ev.Involves(find(app, "zone")); ok {
				sensed = append(sensed, other)
			}
		}
	}))
	run(app, 3)

	rock := find(app, "rock")
	if len(sensed) == 0 || sensed[0] != rock {
		t.Fatalf("zone should sense the falling rock, got %v", sensed)
	}
	if y := translation(app, rock).Y; math.Abs(float64(y-0.3)) > 0.05 {
		t.Fatalf("a sensor must not stop the rock; rock at y=%v", y)
	}

	var p Physics
	p.InitParam(app.World)
	hit, ok := p.CastRayExcluding(rl.Vector3{Y: 10}, rl.Vector3{Y: -1}, 20, find(app, "zone"))
	if !ok || hit.Entity != rock || math.Abs(float64(hit.Distance-9.4)) > 0.05 {
		t.Fatalf("ray should hit the rock top at distance 9.4, got %+v ok=%v", hit, ok)
	}
}

func TestCharacterController(t *testing.T) {
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(tag("hero")), illusion.C(CharacterController{Radius: 0.3, Height: 1.6, StepHeight: 0.3}),
			illusion.C(transform.FromXYZ(0, 2, 0)))
		// A low step and a wall in the way.
		cmd.Spawn(illusion.C(Static), illusion.C(Cuboid(2, 0.2, 4)), illusion.C(transform.FromXYZ(3, 0.1, 0)))
		cmd.Spawn(illusion.C(Static), illusion.C(Cuboid(1, 4, 4)), illusion.C(transform.FromXYZ(7, 2, 0)))
	})
	jumped := false
	app.AddSystems(illusion.FixedUpdate, illusion.Fn1(func(q *illusion.Query1[CharacterController]) {
		q.Each(func(_ ecs.Entity, c *CharacterController) {
			c.Walk = rl.Vector3{X: 3}
			if c.Grounded && !jumped && translation(app, find(app, "hero")).X > 5 {
				c.Velocity.Y = 5
				jumped = true
			}
		})
	}))
	run(app, 1)
	hero := find(app, "hero")
	cc := ecs.NewMap[CharacterController](app.World).Get(hero)
	if !cc.Grounded {
		t.Fatalf("hero should have landed, at %v", translation(app, hero))
	}

	run(app, 3)
	p := translation(app, hero)
	if p.X < 5.8 || p.X > 6.4 {
		t.Fatalf("hero should climb the step and stop at the wall (x≈6.2), at %v", p)
	}
	if !jumped {
		t.Fatal("hero never jumped")
	}
}

func TestCharacterSteppedEveryFewSteps(t *testing.T) {
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(Static), illusion.C(Cuboid(40, 0.2, 40)), illusion.C(transform.FromXYZ(0, -0.1, 0)))
		cmd.Spawn(illusion.C(tag("every")), illusion.C(CharacterController{Radius: 0.3, Height: 1.6}),
			illusion.C(transform.FromXYZ(0, 1, -2)))
		cmd.Spawn(illusion.C(tag("third")), illusion.C(CharacterController{Radius: 0.3, Height: 1.6, Every: 3}),
			illusion.C(transform.FromXYZ(0, 1, 2)))
	})
	stepped, steps := 0, 0
	app.AddSystems(illusion.FixedUpdate, illusion.Fn1(func(q *illusion.Query1[CharacterController]) {
		q.Each(func(_ ecs.Entity, c *CharacterController) { c.Walk = rl.Vector3{X: 1.5} })
	}))
	app.AddSystems(illusion.FixedPostUpdate, illusion.Fn1(func(q *illusion.Query1[CharacterController]) {
		q.Each(func(_ ecs.Entity, c *CharacterController) {
			if c.Every == 3 {
				steps++
				if c.Stepped {
					stepped++
				}
			}
		})
	}))
	run(app, 2)
	if stepped < steps/3-1 || stepped > steps/3+1 {
		t.Fatalf("stepped %d of %d steps, want about a third", stepped, steps)
	}
	every, third := translation(app, find(app, "every")), translation(app, find(app, "third"))
	if math.Abs(float64(every.X-third.X)) > 0.1 {
		t.Fatalf("stepped every third step, it should keep up: at x=%v against x=%v", third.X, every.X)
	}
	if !ecs.NewMap[CharacterController](app.World).Get(find(app, "third")).Grounded {
		t.Fatal("stepped every third step, it should still stand on the ground")
	}
}

func TestCharacterCollisionEvents(t *testing.T) {
	var events []CollisionStarted
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(tag("hero")), illusion.C(CharacterController{Radius: 0.45, Height: 1.3, StepHeight: 0.25}),
			illusion.C(transform.FromXYZ(0, 2, 0)))
		cmd.Spawn(illusion.C(tag("stem")), illusion.C(Static), illusion.C(Cylinder(0.25, 2).WithOffset(rl.Vector3{Y: 1})),
			illusion.C(transform.FromXYZ(3, 0, 0)))
	})
	app.AddSystems(illusion.FixedUpdate, illusion.Fn1(func(q *illusion.Query1[CharacterController]) {
		q.Each(func(_ ecs.Entity, c *CharacterController) { c.Walk = rl.Vector3{X: 3} })
	}))
	app.AddSystems(illusion.Update, illusion.Fn1(func(r *illusion.EventReader[CollisionStarted]) {
		for ev := range r.Read() {
			events = append(events, ev)
		}
	}))
	run(app, 2)

	hero, stem := find(app, "hero"), find(app, "stem")
	if x := translation(app, hero).X; math.Abs(float64(x-2.3)) > 0.05 {
		t.Fatalf("hero should stop against the stem at x≈2.3, got %v", x)
	}
	for _, ev := range events {
		if other, ok := ev.Involves(hero); ok && other == stem {
			if ev.A == hero && ev.Normal.X < 0.9 {
				t.Fatalf("normal should point from hero to stem, got %v", ev.Normal)
			}
			return
		}
	}
	t.Fatalf("expected a hero/stem collision event, got %+v", events)
}

func TestDebugPluginDrawsColliders(t *testing.T) {
	app := illusion.New().AddPlugins(transform.Plugin{}, Plugin{}, diag.Plugin{}, DebugPlugin{})
	app.AddSystems(illusion.Startup, illusion.Fn1(func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(Static), illusion.C(Cuboid(10, 1, 10)), illusion.C(transform.FromXYZ(0, -0.5, 0)))
		cmd.Spawn(illusion.C(Dynamic), illusion.C(Sphere(0.5)), illusion.C(transform.FromXYZ(0, 3, 0)))
		cmd.Spawn(illusion.C(CharacterController{Radius: 0.3, Height: 1.6}), illusion.C(transform.FromXYZ(2, 1, 0)))
	}))
	t.Cleanup(app.Cleanup)

	var gizmoLines int
	app.AddSystems(illusion.Last, illusion.Fn1(func(g *diag.Gizmos) { gizmoLines = g.Count() }))
	run(app, 0.1)
	if gizmoLines == 0 {
		t.Fatal("expected collider outlines")
	}
	// Floor, ball, and the character's inner body.
	if v := ecs.GetResource[diag.Stats](app.World).Get("bodies"); v != "3" {
		t.Fatalf("overlay should report 3 bodies, got %q", v)
	}
}

func TestChangingSettingsRebuildsTheBody(t *testing.T) {
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(tag("crate")), illusion.C(Dynamic), illusion.C(Cuboid(1, 1, 1)),
			illusion.C(transform.FromXYZ(0, 10, 0)))
	})
	run(app, 0.5)
	crate := find(app, "crate")
	falling := translation(app, crate).Y
	if falling >= 10 {
		t.Fatal("crate should be falling")
	}

	// Freeze it mid-air: switching to Static must stop it where it is.
	*ecs.NewMap[RigidBody](app.World).Get(crate) = Static
	run(app, 0.5)
	frozen := translation(app, crate).Y
	run(app, 0.5)
	if y := translation(app, crate).Y; y != frozen || frozen < falling-1 {
		t.Fatalf("static crate should stay put: %v then %v (was falling at %v)", frozen, y, falling)
	}
	var p Physics
	p.InitParam(app.World)
	if hit, ok := p.CastRay(rl.Vector3{X: -5, Y: frozen}, rl.Vector3{X: 1}, 10); !ok || hit.Entity != crate {
		t.Fatal("the rebuilt body should be where the Transform says")
	}

	// Growing the collider: a ray just above the old top should now hit.
	*ecs.NewMap[Collider](app.World).Get(crate) = Cuboid(4, 4, 4)
	run(app, 0.05)
	if _, ok := p.CastRay(rl.Vector3{X: -5, Y: frozen + 1.5}, rl.Vector3{X: 1}, 10); !ok {
		t.Fatal("the new, bigger collider should be used")
	}
	if n := len(ecs.GetResource[world](app.World).entities); n != 2 {
		t.Fatalf("rebuilding must not leak bodies, have %d", n)
	}
}

func TestDespawningATouchingEntityEndsTheCollision(t *testing.T) {
	var ended []CollisionEnded
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(tag("plate")), illusion.C(Static), illusion.C(Sensor{}), illusion.C(Cuboid(2, 0.2, 2)),
			illusion.C(transform.FromXYZ(0, 0.1, 0)))
		cmd.Spawn(illusion.C(tag("crate")), illusion.C(Dynamic), illusion.C(Cuboid(1, 1, 1)),
			illusion.C(transform.FromXYZ(0, 0.5, 0)))
	})
	app.AddSystems(illusion.Update, illusion.Fn1(func(r *illusion.EventReader[CollisionEnded]) {
		for ev := range r.Read() {
			ended = append(ended, ev)
		}
	}))
	run(app, 3) // the crate settles and falls asleep on the plate
	plate, crate := find(app, "plate"), find(app, "crate")
	if len(ended) != 0 {
		t.Fatalf("a crate resting (and sleeping) on the plate must not end the collision: %+v", ended)
	}

	app.AddSystems(illusion.Update, illusion.Fn1(func(cmd *illusion.Commands) { cmd.Despawn(crate) }).RunIf(illusion.Once()))
	run(app, 0.1)
	found := false
	for _, ev := range ended {
		if other, ok := ev.Involves(plate); ok && other == crate {
			found = true
		}
	}
	if !found {
		t.Fatalf("despawning the crate should end its collision with the plate, got %+v", ended)
	}
}

func TestResizingACharacterRebuildsIt(t *testing.T) {
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(tag("hero")), illusion.C(CharacterController{Radius: 0.3, Height: 1.8}),
			illusion.C(transform.FromXYZ(0, 1, 0)))
	})
	run(app, 1)
	hero := find(app, "hero")
	chars := ecs.NewMap[character](app.World)
	before := chars.Get(hero).body

	ecs.NewMap[CharacterController](app.World).Get(hero).Height = 1.0
	run(app, 1)
	after := chars.Get(hero)
	if after.body == before || after.config.height != 1.0 {
		t.Fatalf("changing Height should rebuild the character (body %v -> %v, height %v)", before, after.body, after.config.height)
	}
	// Standing on the floor, a 1m capsule's center sits at y=0.5.
	if y := translation(app, hero).Y; math.Abs(float64(y-0.5)) > 0.05 {
		t.Fatalf("the rebuilt 1m character should stand at y≈0.5, got %v", y)
	}
}

func TestSleepClearsCachedVelocityAndImpulsesWakeTheBody(t *testing.T) {
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(tag("sleeper")), illusion.C(Dynamic), illusion.C(Cuboid(1, 1, 1)),
			illusion.C(transform.FromXYZ(0, .5, 0)), illusion.C(Mass(10)), illusion.C(Velocity{Linear: rl.Vector3{X: .1}}))
	})
	run(app, .1)
	e := find(app, "sleeper")
	var p Physics
	p.InitParam(app.World)
	p.Sleep(e)
	if !p.Asleep(e) || *ecs.NewMap[Velocity](app.World).Get(e) != (Velocity{}) {
		t.Fatal("Sleep did not clear body and component velocity")
	}
	before := translation(app, e)
	run(app, .5)
	if !p.Asleep(e) || translation(app, e) != before {
		t.Fatal("Prepare reapplied stale velocity and woke the body")
	}
	p.AddImpulse(e, rl.Vector3{X: 50, Y: 20})
	run(app, .2)
	if p.Asleep(e) || translation(app, e).X < before.X+.5 {
		t.Fatal("a later impact did not wake the body")
	}
}
