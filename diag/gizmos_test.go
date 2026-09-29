package diag

import (
	"math"
	"testing"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/struckchure/illusion"
)

func newGizmos() *Gizmos { return &Gizmos{buf: &gizmoBuffer{}} }

func TestBoxHasTwelveEdgesAtTheRightCorners(t *testing.T) {
	g := newGizmos()
	m := rl.MatrixMultiply(rl.MatrixRotateY(math.Pi/2), rl.MatrixTranslate(10, 0, 0))
	g.Box(m, rl.Vector3{X: 1, Y: 2, Z: 3}, rl.White)
	if len(g.buf.frame.lines3) != 12 {
		t.Fatalf("expected 12 edges, got %d", len(g.buf.frame.lines3))
	}
	for _, l := range g.buf.frame.lines3 {
		for _, p := range []rl.Vector3{l.a, l.b} {
			// Rotated 90° about Y, the box's X/Z extents swap around (10, 0, 0).
			if math.Abs(float64(math.Abs(float64(p.X-10))-3)) > 1e-4 ||
				math.Abs(math.Abs(float64(p.Y))-2) > 1e-4 ||
				math.Abs(math.Abs(float64(p.Z))-1) > 1e-4 {
				t.Fatalf("corner %v is off the box", p)
			}
		}
	}
}

func TestCirclePointsLieOnTheCircle(t *testing.T) {
	g := newGizmos()
	center := rl.Vector3{X: 1, Y: 2, Z: 3}
	g.Circle(center, rl.Vector3{X: 1, Y: 1}, 2, rl.White)
	if len(g.buf.frame.lines3) != circleSegments {
		t.Fatalf("segments %d", len(g.buf.frame.lines3))
	}
	for _, l := range g.buf.frame.lines3 {
		if d := rl.Vector3Distance(l.a, center); math.Abs(float64(d-2)) > 1e-4 {
			t.Fatalf("point %v at distance %v", l.a, d)
		}
	}
}

func TestRect2DAndClear(t *testing.T) {
	g := newGizmos()
	g.Rect2D(rl.Vector2{X: 5, Y: 5}, rl.Vector2{X: 4, Y: 2}, 0, rl.White)
	if len(g.buf.frame.lines2) != 4 || g.buf.frame.lines2[0].a != (rl.Vector2{X: 3, Y: 4}) {
		t.Fatalf("rect lines %v", g.buf.frame.lines2)
	}
	g.Capsule(rl.MatrixIdentity(), 0.5, 1, rl.White)
	if len(g.buf.frame.lines3) == 0 {
		t.Fatal("capsule drew nothing")
	}
}

func TestFixedStepGizmosPersistBetweenSteps(t *testing.T) {
	app := illusion.New()
	buildGizmos(app)
	app.AddSystems(illusion.FixedUpdate, illusion.Fn1(func(g *Gizmos) {
		g.Line(rl.Vector3{}, rl.Vector3{X: 1}, rl.White)
	}))
	app.AddSystems(illusion.Update, illusion.Fn1(func(g *Gizmos) {
		g.Line(rl.Vector3{}, rl.Vector3{Y: 1}, rl.White)
	}))
	var g Gizmos
	g.InitParam(app.World)

	app.Tick(time.Second / 60) // one fixed step has run

	// At 120 FPS only every other frame runs the 60 Hz fixed step; at 30 FPS
	// every frame runs two. Either way one fixed line and one frame line show.
	for _, dt := range []time.Duration{time.Second / 120, time.Second / 120, time.Second / 120, time.Second / 30} {
		app.Tick(dt)
		if n := g.Count(); n != 2 {
			t.Fatalf("after a %v frame: %d lines, want 2", dt, n)
		}
	}
}
