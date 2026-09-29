package physics

import (
	"fmt"
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/diag"
	"github.com/struckchure/illusion/input"
	"github.com/struckchure/illusion/transform"
)

// Collider outline colors used by DebugPlugin.
var (
	DebugDynamic   = color.RGBA{R: 80, G: 230, B: 120, A: 255}
	DebugSleeping  = color.RGBA{R: 150, G: 150, B: 150, A: 255}
	DebugStatic    = color.RGBA{R: 90, G: 150, B: 255, A: 255}
	DebugKinematic = color.RGBA{R: 255, G: 210, B: 60, A: 255}
	DebugSensor    = color.RGBA{R: 255, G: 90, B: 220, A: 255}
	DebugCharacter = color.RGBA{R: 60, G: 230, B: 230, A: 255}
)

// DebugSettings is a resource controlling collider outlines.
type DebugSettings struct {
	Enabled bool
}

// DebugPlugin outlines every collider and character with gizmos, and reports
// the body count on the diag overlay. It needs diag.Plugin.
type DebugPlugin struct {
	// Hidden starts with the outlines off.
	Hidden bool
	// ToggleKey shows and hides the outlines; 0 means F4.
	ToggleKey int32
}

// Build implements [illusion.Plugin].
func (p DebugPlugin) Build(app *illusion.App) {
	key := p.ToggleKey
	if key == 0 {
		key = rl.KeyF4
	}
	app.InitResource(illusion.R(&DebugSettings{Enabled: !p.Hidden}))
	app.AddSystems(illusion.Update, illusion.Fn2(func(keys *illusion.Res[input.Keys], s *illusion.Res[DebugSettings]) {
		if keys.Get().JustPressed(input.Key(key)) {
			s.Get().Enabled = !s.Get().Enabled
		}
	}).RunIf(illusion.ResourceExists[input.Keys]()).Named("physics.toggleDebug"))
	app.AddSystems(illusion.PostUpdate, illusion.Fn5(drawColliders).
		After(transform.Propagate).
		RunIf(illusion.Cond1(func(s *illusion.Res[DebugSettings]) bool { return s.Get().Enabled })).
		Named("physics.drawColliders"))
	app.AddSystems(illusion.PostUpdate, illusion.Fn2(reportBodies).Named("physics.reportBodies"))
}

func drawColliders(
	bodies *illusion.Query4[body, RigidBody, Collider, transform.Transform],
	characters *illusion.Query2[CharacterController, transform.Transform],
	sensors *illusion.Query0Where[illusion.With[Sensor]],
	res *illusion.Res[world],
	g *diag.Gizmos,
) {
	w := res.Get()
	query := bodies.Iter()
	for query.Next() {
		b, rb, col, tr := query.Get()
		c := DebugDynamic
		switch {
		case sensors.Contains(query.Entity()):
			c = DebugSensor
		case *rb == Static:
			c = DebugStatic
		case *rb == Kinematic:
			c = DebugKinematic
		case !w.jolt.IsActive(b.id):
			c = DebugSleeping
		}
		drawCollider(g, col, bodyMatrix(tr), c)
	}
	characters.Each(func(_ ecs.Entity, cc *CharacterController, tr *transform.Transform) {
		radius := cc.Radius
		if radius <= 0 {
			radius = 0.3
		}
		height := max(cc.Height, 2*radius+0.01)
		m := rl.MatrixTranslate(tr.Translation.X, tr.Translation.Y, tr.Translation.Z)
		g.Capsule(m, radius, height/2-radius, DebugCharacter)
		if cc.Grounded {
			g.Arrow(tr.Translation, rl.Vector3Add(tr.Translation, cc.GroundNormal), DebugCharacter)
		}
	})
}

// bodyMatrix is the body's pose without scale (colliders ignore it).
func bodyMatrix(tr *transform.Transform) rl.Matrix {
	pose := *tr
	pose.Scale = rl.Vector3One()
	return pose.Matrix()
}

func drawCollider(g *diag.Gizmos, col *Collider, m rl.Matrix, c color.RGBA) {
	if col.hasOffset {
		m = rl.MatrixMultiply(rl.MatrixTranslate(col.offset.X, col.offset.Y, col.offset.Z), m)
	}
	switch col.kind {
	case shapeBox:
		g.Box(m, vec(col.size), c)
	case shapeSphere:
		center := rl.Vector3Transform(rl.Vector3{}, m)
		g.Sphere(center, col.size[0], c)
	case shapeCapsule:
		g.Capsule(m, col.size[0], col.size[1], c)
	case shapeCylinder:
		g.Cylinder(m, col.size[0], col.size[1], c)
	case shapeConvexHull:
		lo, hi := bounds(col.points)
		center := rl.Vector3Scale(rl.Vector3Add(lo, hi), 0.5)
		g.Box(rl.MatrixMultiply(rl.MatrixTranslate(center.X, center.Y, center.Z), m),
			rl.Vector3Scale(rl.Vector3Subtract(hi, lo), 0.5), c)
	case shapeMesh:
		// Cap the cost for big level meshes.
		const maxTriangles = 20000
		for i := 0; i+2 < len(col.indices) && i/3 < maxTriangles; i += 3 {
			a := rl.Vector3Transform(vec(col.points[col.indices[i]]), m)
			b := rl.Vector3Transform(vec(col.points[col.indices[i+1]]), m)
			d := rl.Vector3Transform(vec(col.points[col.indices[i+2]]), m)
			g.Line(a, b, c)
			g.Line(b, d, c)
			g.Line(d, a, c)
		}
	}
}

func bounds(points [][3]float32) (lo, hi rl.Vector3) {
	if len(points) == 0 {
		return lo, hi
	}
	lo, hi = vec(points[0]), vec(points[0])
	for _, p := range points[1:] {
		v := vec(p)
		lo = rl.Vector3Min(lo, v)
		hi = rl.Vector3Max(hi, v)
	}
	return lo, hi
}

func reportBodies(res *illusion.Res[world], stats *illusion.Res[diag.Stats]) {
	if s, ok := stats.TryGet(); ok {
		w := res.Get()
		s.Set("bodies", fmt.Sprint(len(w.entities)-len(w.removed)))
	}
}
