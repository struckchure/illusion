// Package diag provides debugging aids: gizmos (immediate-mode debug shapes
// you can emit from any system) and an on-screen stats overlay.
package diag

import (
	"image/color"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/render"
)

const circleSegments = 24

type line3 struct {
	a, b rl.Vector3
	c    color.RGBA
}

type line2 struct {
	a, b rl.Vector2
	c    color.RGBA
}

// gizmoBuffer holds the shapes emitted this frame.
// gizmoLayer is one batch of lines.
type gizmoLayer struct {
	lines3 []line3
	lines2 []line2
}

func (l *gizmoLayer) clear() {
	l.lines3 = l.lines3[:0]
	l.lines2 = l.lines2[:0]
}

// gizmoBuffer holds shapes emitted during the frame, cleared every frame, and
// shapes emitted during fixed steps, cleared every fixed step. Fixed shapes
// stay visible on frames that run no fixed step, and only the last step's
// shapes show on frames that run several.
type gizmoBuffer struct {
	frame, fixed gizmoLayer
}

func (b *gizmoBuffer) count() int {
	return len(b.frame.lines3) + len(b.frame.lines2) + len(b.fixed.lines3) + len(b.fixed.lines2)
}

// Gizmos is a system parameter for drawing debug shapes. Shapes are drawn on
// the current frame and then forgotten, so emit them every frame you want
// them visible. 3D shapes appear through the 3D camera and 2D shapes through
// the 2D camera.
//
// Shapes emitted from Fixed* schedules last until the next fixed step
// instead, so they neither flicker nor double up when the frame rate and the
// fixed rate differ.
type Gizmos struct {
	buf  *gizmoBuffer
	time *illusion.Time
}

// InitParam implements [illusion.Param].
func (g *Gizmos) InitParam(w *ecs.World) {
	g.buf = ecs.GetResource[gizmoBuffer](w)
	if g.buf == nil {
		panic("diag: Gizmos used without diag.Plugin")
	}
	g.time = ecs.GetResource[illusion.Time](w)
}

// layer is where shapes emitted right now go.
func (g *Gizmos) layer() *gizmoLayer {
	if g.time != nil && g.time.InFixedStep() {
		return &g.buf.fixed
	}
	return &g.buf.frame
}

// Count returns the number of line segments that will be drawn this frame.
func (g *Gizmos) Count() int { return g.buf.count() }

// Line draws a 3D line segment.
func (g *Gizmos) Line(a, b rl.Vector3, c color.RGBA) {
	l := g.layer()
	l.lines3 = append(l.lines3, line3{a, b, c})
}

// Ray draws a line from origin along direction (whose length is the ray's).
func (g *Gizmos) Ray(origin, direction rl.Vector3, c color.RGBA) {
	g.Line(origin, rl.Vector3Add(origin, direction), c)
}

// Arrow draws a line from a to b with a head at b.
func (g *Gizmos) Arrow(a, b rl.Vector3, c color.RGBA) {
	g.Line(a, b, c)
	dir := rl.Vector3Subtract(b, a)
	length := rl.Vector3Length(dir)
	if length == 0 {
		return
	}
	dir = rl.Vector3Scale(dir, 1/length)
	u, v := basis(dir)
	head := length * 0.15
	back := rl.Vector3Subtract(b, rl.Vector3Scale(dir, head))
	for _, side := range []rl.Vector3{u, v, rl.Vector3Negate(u), rl.Vector3Negate(v)} {
		g.Line(b, rl.Vector3Add(back, rl.Vector3Scale(side, head*0.5)), c)
	}
}

// Circle draws a circle facing normal.
func (g *Gizmos) Circle(center, normal rl.Vector3, radius float32, c color.RGBA) {
	u, v := basis(rl.Vector3Normalize(normal))
	g.arc(center, u, v, radius, 0, 2*math.Pi, c)
}

// Sphere draws a sphere as three circles.
func (g *Gizmos) Sphere(center rl.Vector3, radius float32, c color.RGBA) {
	g.Circle(center, rl.Vector3{X: 1}, radius, c)
	g.Circle(center, rl.Vector3{Y: 1}, radius, c)
	g.Circle(center, rl.Vector3{Z: 1}, radius, c)
}

// Box draws a box with the given half extents, placed by the world matrix m
// (e.g. a GlobalTransform's Matrix).
func (g *Gizmos) Box(m rl.Matrix, halfExtents rl.Vector3, c color.RGBA) {
	h := halfExtents
	var p [8]rl.Vector3
	for i := range p {
		corner := rl.Vector3{X: h.X, Y: h.Y, Z: h.Z}
		if i&1 != 0 {
			corner.X = -h.X
		}
		if i&2 != 0 {
			corner.Y = -h.Y
		}
		if i&4 != 0 {
			corner.Z = -h.Z
		}
		p[i] = rl.Vector3Transform(corner, m)
	}
	for i := range p {
		for _, bit := range []int{1, 2, 4} {
			if j := i | bit; j != i {
				g.Line(p[i], p[j], c)
			}
		}
	}
}

// Cuboid draws an axis-aligned box centered on center with the given full size.
func (g *Gizmos) Cuboid(center, size rl.Vector3, c color.RGBA) {
	g.Box(rl.MatrixTranslate(center.X, center.Y, center.Z), rl.Vector3Scale(size, 0.5), c)
}

// Cylinder draws an upright cylinder, placed by the world matrix m.
func (g *Gizmos) Cylinder(m rl.Matrix, radius, halfHeight float32, c color.RGBA) {
	x, y, z := axes(m)
	center := origin(m)
	top, bottom := rl.Vector3Add(center, rl.Vector3Scale(y, halfHeight)), rl.Vector3Subtract(center, rl.Vector3Scale(y, halfHeight))
	g.arc(top, x, z, radius, 0, 2*math.Pi, c)
	g.arc(bottom, x, z, radius, 0, 2*math.Pi, c)
	for _, side := range []rl.Vector3{x, z, rl.Vector3Negate(x), rl.Vector3Negate(z)} {
		off := rl.Vector3Scale(side, radius)
		g.Line(rl.Vector3Add(top, off), rl.Vector3Add(bottom, off), c)
	}
}

// Capsule draws an upright capsule; halfHeight is half the straight part.
func (g *Gizmos) Capsule(m rl.Matrix, radius, halfHeight float32, c color.RGBA) {
	x, y, z := axes(m)
	g.Cylinder(m, radius, halfHeight, c)
	center := origin(m)
	top, bottom := rl.Vector3Add(center, rl.Vector3Scale(y, halfHeight)), rl.Vector3Subtract(center, rl.Vector3Scale(y, halfHeight))
	for _, side := range []rl.Vector3{x, z} {
		g.arc(top, side, y, radius, 0, math.Pi, c)
		g.arc(bottom, side, y, radius, math.Pi, 2*math.Pi, c)
	}
}

// Axes draws the X (red), Y (green) and Z (blue) axes of a world matrix.
func (g *Gizmos) Axes(m rl.Matrix, length float32) {
	x, y, z := axes(m)
	o := origin(m)
	g.Arrow(o, rl.Vector3Add(o, rl.Vector3Scale(x, length)), rl.Red)
	g.Arrow(o, rl.Vector3Add(o, rl.Vector3Scale(y, length)), rl.Green)
	g.Arrow(o, rl.Vector3Add(o, rl.Vector3Scale(z, length)), rl.Blue)
}

// Line2D draws a line in the 2D world.
func (g *Gizmos) Line2D(a, b rl.Vector2, c color.RGBA) {
	l := g.layer()
	l.lines2 = append(l.lines2, line2{a, b, c})
}

// Circle2D draws a circle in the 2D world.
func (g *Gizmos) Circle2D(center rl.Vector2, radius float32, c color.RGBA) {
	prev := rl.Vector2{X: center.X + radius, Y: center.Y}
	for i := 1; i <= circleSegments; i++ {
		a := float64(i) / circleSegments * 2 * math.Pi
		p := rl.Vector2{X: center.X + radius*float32(math.Cos(a)), Y: center.Y + radius*float32(math.Sin(a))}
		g.Line2D(prev, p, c)
		prev = p
	}
}

// Rect2D draws a rectangle in the 2D world, rotated by angle radians around
// its center.
func (g *Gizmos) Rect2D(center, size rl.Vector2, angle float32, c color.RGBA) {
	sin, cos := math.Sincos(float64(angle))
	corner := func(x, y float32) rl.Vector2 {
		return rl.Vector2{
			X: center.X + x*float32(cos) - y*float32(sin),
			Y: center.Y + x*float32(sin) + y*float32(cos),
		}
	}
	hx, hy := size.X/2, size.Y/2
	p := [4]rl.Vector2{corner(-hx, -hy), corner(hx, -hy), corner(hx, hy), corner(-hx, hy)}
	for i := range p {
		g.Line2D(p[i], p[(i+1)%4], c)
	}
}

// arc draws part of a circle in the plane spanned by u and v.
func (g *Gizmos) arc(center, u, v rl.Vector3, radius float32, from, to float64, c color.RGBA) {
	segments := max(2, int(math.Ceil(circleSegments*(to-from)/(2*math.Pi))))
	point := func(a float64) rl.Vector3 {
		return rl.Vector3Add(center, rl.Vector3Add(
			rl.Vector3Scale(u, radius*float32(math.Cos(a))),
			rl.Vector3Scale(v, radius*float32(math.Sin(a)))))
	}
	prev := point(from)
	for i := 1; i <= segments; i++ {
		p := point(from + (to-from)*float64(i)/float64(segments))
		g.Line(prev, p, c)
		prev = p
	}
}

// basis returns two unit vectors perpendicular to n and to each other.
func basis(n rl.Vector3) (u, v rl.Vector3) {
	ref := rl.Vector3{Y: 1}
	if math.Abs(float64(n.Y)) > 0.9 {
		ref = rl.Vector3{X: 1}
	}
	u = rl.Vector3Normalize(rl.Vector3CrossProduct(n, ref))
	v = rl.Vector3CrossProduct(n, u)
	return u, v
}

// axes returns a matrix's unit X, Y and Z axes.
func axes(m rl.Matrix) (x, y, z rl.Vector3) {
	return rl.Vector3Normalize(rl.Vector3{X: m.M0, Y: m.M1, Z: m.M2}),
		rl.Vector3Normalize(rl.Vector3{X: m.M4, Y: m.M5, Z: m.M6}),
		rl.Vector3Normalize(rl.Vector3{X: m.M8, Y: m.M9, Z: m.M10})
}

func origin(m rl.Matrix) rl.Vector3 { return rl.Vector3{X: m.M12, Y: m.M13, Z: m.M14} }

func drawGizmos3D(buf *illusion.Res[gizmoBuffer], view *illusion.Res[render.View3D]) {
	if v, ok := view.TryGet(); !ok || !v.Active {
		return
	}
	b := buf.Get()
	for _, layer := range []*gizmoLayer{&b.fixed, &b.frame} {
		for _, l := range layer.lines3 {
			rl.DrawLine3D(l.a, l.b, l.c)
		}
	}
}

func drawGizmos2D(buf *illusion.Res[gizmoBuffer], view *illusion.Res[render.View2D]) {
	if v, ok := view.TryGet(); !ok || !v.Active {
		return
	}
	b := buf.Get()
	for _, layer := range []*gizmoLayer{&b.fixed, &b.frame} {
		for _, l := range layer.lines2 {
			rl.DrawLineV(l.a, l.b, l.c)
		}
	}
}

func clearGizmos(buf *illusion.Res[gizmoBuffer])      { buf.Get().frame.clear() }
func clearFixedGizmos(buf *illusion.Res[gizmoBuffer]) { buf.Get().fixed.clear() }

func buildGizmos(app *illusion.App) {
	app.InsertResource(illusion.R(&gizmoBuffer{}))
	// Shapes emitted during a frame are drawn in its Render schedule and
	// cleared at the start of the next frame.
	app.AddSystems(illusion.First, illusion.Fn1(clearGizmos).Named("diag.clearGizmos"))
	app.AddSystems(illusion.FixedFirst, illusion.Fn1(clearFixedGizmos).Named("diag.clearFixedGizmos"))
	app.AddSystems(illusion.Render,
		illusion.Fn2(drawGizmos3D).InSet(render.Draw3D).Named("diag.gizmos3D"),
		illusion.Fn2(drawGizmos2D).InSet(render.DrawWorld2D).Named("diag.gizmos2D"),
	)
}
