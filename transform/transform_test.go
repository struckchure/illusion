package transform

import (
	"math"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
)

func near(a, b rl.Vector3) bool {
	return rl.Vector3Distance(a, b) < 1e-4
}

func TestLookAt(t *testing.T) {
	cases := []rl.Vector3{
		{X: 0, Y: 0, Z: -5},
		{X: 3, Y: 0, Z: 0},
		{X: 1, Y: 2, Z: 3},
		{X: 0, Y: -4, Z: 0.0001},
		{X: 0, Y: 7, Z: 0}, // parallel to up
	}
	for _, target := range cases {
		tr := FromXYZ(0, 0, 0).LookingAt(target, Up)
		want := rl.Vector3Normalize(target)
		if got := tr.Forward(); !near(got, want) {
			t.Errorf("LookAt(%v): forward %v, want %v", target, got, want)
		}
	}
}

func TestRotateY(t *testing.T) {
	tr := Identity()
	tr.RotateY(math.Pi / 2)
	if got := tr.Forward(); !near(got, rl.Vector3{X: -1}) {
		t.Fatalf("forward after 90° yaw: %v", got)
	}
}

func TestMatrixMatchesTransform(t *testing.T) {
	tr := FromXYZ(1, 2, 3).WithScale(2)
	tr.RotateY(math.Pi / 2)
	g := GlobalTransform{Matrix: tr.Matrix()}

	if !near(g.Translation(), tr.Translation) {
		t.Fatalf("translation %v", g.Translation())
	}
	if !near(g.Forward(), tr.Forward()) || !near(g.Up(), tr.Up()) {
		t.Fatalf("axes: forward %v up %v", g.Forward(), g.Up())
	}
	// A point one unit in front of the entity, in local space, should land
	// two units (scale 2) in front of it in world space.
	p := rl.Vector3Transform(rl.Vector3{Z: -1}, g.Matrix)
	if want := rl.Vector3Add(tr.Translation, rl.Vector3Scale(tr.Forward(), 2)); !near(p, want) {
		t.Fatalf("point %v, want %v", p, want)
	}
	back := g.Decompose()
	if !near(back.Scale, tr.Scale) || !near(back.Translation, tr.Translation) {
		t.Fatalf("decompose %+v", back)
	}
}

func TestZeroRotationIsIdentity(t *testing.T) {
	tr := Transform{Scale: rl.Vector3One()}
	if !near(tr.Forward(), Forward) {
		t.Fatalf("forward %v", tr.Forward())
	}
}

func TestPluginAddsAndUpdatesGlobalTransform(t *testing.T) {
	var q illusion.Query2[Transform, GlobalTransform]
	app := illusion.New().
		AddPlugins(Plugin{}).
		AddSystems(illusion.Startup, illusion.Fn1(func(cmd *illusion.Commands) {
			cmd.Spawn(illusion.C(FromXYZ(1, 2, 3)))
		})).
		AddSystems(illusion.Update, illusion.Fn1(func(q *illusion.Query1[Transform]) {
			q.Each(func(_ ecs.Entity, t *Transform) { t.Translation.X += 1 })
		}))

	app.Tick(0)
	q.InitParam(app.World)
	_, _, g, ok := q.Single()
	if !ok {
		t.Fatal("GlobalTransform was not added")
	}
	if got := g.Translation(); !near(got, rl.Vector3{X: 2, Y: 2, Z: 3}) {
		t.Fatalf("global translation %v", got)
	}
}

func TestHierarchyPropagation(t *testing.T) {
	var child ecs.Entity
	app := illusion.New().
		AddPlugins(Plugin{}).
		AddSystems(illusion.Startup, illusion.Fn1(func(cmd *illusion.Commands) {
			parent := FromXYZ(10, 0, 0).WithScale(2)
			parent.RotateY(math.Pi / 2) // parent's -Z now points along world -X
			cmd.Spawn(illusion.C(parent)).WithChildren(func(c *illusion.ChildBuilder) {
				c.Spawn(illusion.C(Identity())).WithChildren(func(c *illusion.ChildBuilder) {
					c.Spawn(illusion.C(FromXYZ(0, 0, -1))).Then(func(_ *ecs.World, e ecs.Entity) { child = e })
				})
			})
		}))
	app.Tick(0)

	g := ecs.NewMap[GlobalTransform](app.World).Get(child)
	// One unit forward in the parent's frame, scaled by 2: world (8, 0, 0).
	if got := g.Translation(); !near(got, rl.Vector3{X: 8}) {
		t.Fatalf("grandchild at %v, want (8, 0, 0)", got)
	}
	if got := g.Forward(); !near(got, rl.Vector3{X: -1}) {
		t.Fatalf("grandchild forward %v", got)
	}
}
