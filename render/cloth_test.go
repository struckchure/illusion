package render

import (
	"math"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// strip is a vertical ribbon of quads hanging from its top two vertices,
// with the bottom row's vertices duplicated (as a texture seam splits them).
func strip(rows int) ([]rl.Vector3, []int32) {
	var v []rl.Vector3
	for r := range rows + 1 {
		y := -float32(r) * 0.1
		v = append(v, rl.Vector3{X: 0, Y: y}, rl.Vector3{X: 0.1, Y: y})
	}
	var t []int32
	for r := range rows {
		a := int32(2 * r)
		t = append(t, a, a+2, a+1, a+1, a+2, a+3)
	}
	// The seam: the last row again, used by no triangle.
	v = append(v, v[2*rows], v[2*rows+1])
	return v, t
}

func TestClothMeshWeldsSeams(t *testing.T) {
	v, tri := strip(2)
	freedom := make([]float32, len(v))
	freedom[len(v)-1] = 0.3 // only the duplicate says the corner moves
	c := newClothMesh(v, tri, freedom)
	if len(c.first) != 6 {
		t.Fatalf("%d particles, want 6 (the seam's two welded)", len(c.first))
	}
	corner := c.particle[5]
	if c.particle[len(v)-1] != corner || c.freedom[corner] != 0.3 {
		t.Errorf("the seam copy isn't the corner's particle, or its freedom was lost: %v", c.freedom)
	}
	if len(c.free) != 1 || c.free[0] != corner {
		t.Errorf("free = %v, want just the corner", c.free)
	}
}

// hang steps c at rest (targets where the vertices are, offset by shift)
// for n frames of one step each.
func hang(c *clothMesh, v []rl.Vector3, shift rl.Vector3, n int, colliders []worldCapsule) {
	for p, i := range c.first {
		c.target[p] = rl.Vector3Add(v[i], shift)
	}
	if !c.started {
		copy(c.pos, c.target)
		copy(c.prev, c.target)
		copy(c.lastTarget, c.target)
		c.started = true
	}
	for range n {
		c.step(clothStep, 1, rl.Vector3{Y: -9.8}, 0.97, 0.02, 0.5, colliders)
		copy(c.lastTarget, c.target)
	}
}

func TestClothStaysWithinFreedom(t *testing.T) {
	v, tri := strip(4)
	freedom := ClothFreedom(v, tri, []bool{true, true}, 0.5, 0.2)
	c := newClothMesh(v, tri, freedom)
	hang(c, v, rl.Vector3{}, 1, nil)
	// The body jumps half a metre sideways: the free end trails behind,
	// but no further than its freedom.
	hang(c, v, rl.Vector3{X: 0.5}, 1, nil)
	bottom := c.particle[8]
	target := c.target[bottom]
	lag := rl.Vector3Distance(c.pos[bottom], target)
	if lag == 0 {
		t.Error("the free end moved with the body at once")
	}
	if lag > c.freedom[bottom]+1e-4 {
		t.Errorf("the free end is %.3f from its place, beyond its freedom %.3f", lag, c.freedom[bottom])
	}
	for p := range c.first {
		if c.freedom[p] == 0 && c.at(int32(p), 1) != c.target[p] {
			t.Errorf("pinned particle %d moved", p)
		}
	}
	// Left to settle, it comes back to hang near its place.
	hang(c, v, rl.Vector3{X: 0.5}, 600, nil)
	if d := rl.Vector3Distance(c.pos[bottom], target); d > 0.05 {
		t.Errorf("after settling the free end is %.3f from its place", d)
	}
}

func TestClothKeepsItsEdges(t *testing.T) {
	v, tri := strip(4)
	c := newClothMesh(v, tri, ClothFreedom(v, tri, []bool{true, true}, 1, 1))
	hang(c, v, rl.Vector3{}, 1, nil)
	hang(c, v, rl.Vector3{Z: 0.3}, 30, nil)
	for _, e := range c.edges {
		got := rl.Vector3Distance(c.at(e[0], 1), c.at(e[1], 1))
		want := rl.Vector3Distance(c.target[e[0]], c.target[e[1]])
		if math.Abs(float64(got-want)) > 0.02 {
			t.Errorf("edge %v is %.3f long, want about %.3f", e, got, want)
		}
	}
}

func TestCollidePushesOut(t *testing.T) {
	k := []worldCapsule{{a: rl.Vector3{}, b: rl.Vector3{Y: 1}, radius: 0.1}}
	// A point inside, whose place is outside: out to the surface.
	got := collide(rl.Vector3{X: 0.05, Y: 0.5}, rl.Vector3{X: 0.2, Y: 0.5}, k)
	if math.Abs(float64(got.X-0.1)) > 1e-5 || got.Y != 0.5 {
		t.Errorf("pushed to %v, want (0.1, 0.5, 0)", got)
	}
	// Its place is inside the capsule too (the pose sank into the body):
	// out all the same.
	got = collide(rl.Vector3{X: 0.02, Y: 0.5}, rl.Vector3{X: 0.06, Y: 0.5}, k)
	if math.Abs(float64(got.X-0.1)) > 1e-5 {
		t.Errorf("pushed to %v, want x 0.1, the surface", got)
	}
	// On the axis: out towards its place.
	got = collide(rl.Vector3{Y: 0.5}, rl.Vector3{Z: -0.3, Y: 0.5}, k)
	if math.Abs(float64(got.Z+0.1)) > 1e-5 {
		t.Errorf("pushed to %v, want z -0.1", got)
	}
	// Outside already: untouched.
	p := rl.Vector3{X: 0.3, Y: 0.5}
	if got := collide(p, p, k); got != p {
		t.Errorf("an outside point moved to %v", got)
	}
}

func TestClothFreedomGrowsAlongTheMesh(t *testing.T) {
	v, tri := strip(3)
	f := ClothFreedom(v, tri, []bool{true, true}, 0.5, 0.12)
	if f[0] != 0 || f[1] != 0 {
		t.Errorf("pinned vertices have freedom %v %v", f[0], f[1])
	}
	if math.Abs(float64(f[2]-0.05)) > 1e-5 {
		t.Errorf("one row down: %v, want 0.05", f[2])
	}
	if f[6] != 0.12 || f[len(v)-2] != 0.12 {
		t.Errorf("the bottom row: %v and its seam copy %v, want the cap 0.12", f[6], f[len(v)-2])
	}
	if none := ClothFreedom(v, tri, nil, 1, 1); none[6] != 0 {
		t.Errorf("with nothing pinned, freedom %v, want none", none[6])
	}
}

func TestPlaceColliders(t *testing.T) {
	up := rl.MatrixTranslate(0, 1, 0)
	bones := []rl.Matrix{rl.MatrixIdentity(), up}
	got := placeColliders([]Capsule{{A: rl.Vector3{X: 1}, B: rl.Vector3{X: 2}, BoneA: 0, BoneB: 1, Radius: 0.1}, {BoneA: 5}}, bones, rl.MatrixTranslate(0, 0, 3), 0.01)
	if len(got) != 1 {
		t.Fatalf("%d capsules, want 1 (the one with a missing bone dropped)", len(got))
	}
	if got[0].a != (rl.Vector3{X: 1, Z: 3}) || got[0].b != (rl.Vector3{X: 2, Y: 1, Z: 3}) || math.Abs(float64(got[0].radius-0.11)) > 1e-6 {
		t.Errorf("placed %+v", got[0])
	}
}

func TestClothNeverGoesBehindItsPlace(t *testing.T) {
	v, tri := strip(4)
	c := newClothMesh(v, tri, ClothFreedom(v, tri, []bool{true, true}, 1, 1))
	for _, p := range c.free {
		c.normal[p] = rl.Vector3{Z: 1} // the surface faces +Z
	}
	hang(c, v, rl.Vector3{}, 1, nil)
	// The body lurches forward (+Z): the cloth would trail behind it, into
	// the surface, but stays in front.
	for i := range 20 {
		hang(c, v, rl.Vector3{Z: 0.05 * float32(i)}, 1, nil)
		for _, p := range c.free {
			if d := c.pos[p].Z - c.target[p].Z; d < -1e-5 {
				t.Fatalf("frame %d: particle %d is %.3f behind its place", i, p, -d)
			}
		}
	}
}

func TestClothIsPushedOutOfTheBody(t *testing.T) {
	v, tri := strip(4)
	c := newClothMesh(v, tri, ClothFreedom(v, tri, []bool{true, true}, 1, 1))
	// A leg swings into the cloth: the pose puts the bottom of the strip
	// inside it, but the cloth stays out.
	leg := []worldCapsule{{a: rl.Vector3{X: 0.05, Y: -0.5, Z: -0.1}, b: rl.Vector3{X: 0.05, Y: -0.5, Z: 0.1}, radius: 0.08}}
	hang(c, v, rl.Vector3{}, 1, leg)
	hang(c, v, rl.Vector3{}, 60, leg)
	for _, p := range c.free {
		if d := rl.Vector3Distance(c.pos[p], closestOnSegment(c.pos[p], leg[0].a, leg[0].b)); d < leg[0].radius-1e-4 {
			t.Errorf("particle %d is %.3f inside the leg", p, leg[0].radius-d)
		}
	}
}

func TestClothRestsWhereItHangs(t *testing.T) {
	// Left alone, a hanging strip stays as it's modelled rather than
	// sagging below it: the model is how it hangs.
	v, tri := strip(4)
	c := newClothMesh(v, tri, ClothFreedom(v, tri, []bool{true, true}, 1, 0.3))
	hang(c, v, rl.Vector3{}, 600, nil)
	for p := range c.first {
		if d := rl.Vector3Distance(c.pos[p], c.target[p]); d > 0.002 {
			t.Errorf("particle %d rests %.4f from where it hangs", p, d)
		}
	}
}

func TestClothBendsBetweenTriangles(t *testing.T) {
	// The strip's quads are two triangles each, and neighbouring triangles
	// share edges: each shared edge with a moving far corner is a bend.
	v, tri := strip(2)
	c := newClothMesh(v, tri, ClothFreedom(v, tri, []bool{true, true}, 1, 1))
	if len(c.bends) != 3 {
		t.Fatalf("%d bends, want 3 (one per shared edge): %v", len(c.bends), c.bends)
	}
	for _, b := range c.bends {
		if b[0] == b[1] {
			t.Errorf("bend %v joins a particle to itself", b)
		}
	}
}

func TestReachFindsEveryColliderAParticleTouches(t *testing.T) {
	// A strip of cloth hanging past three limbs.
	var vertices []rl.Vector3
	var triangles []int32
	for i := range 20 {
		y := float32(i) * 0.05
		vertices = append(vertices, rl.Vector3{X: -0.05, Y: y}, rl.Vector3{X: 0.05, Y: y})
		if i > 0 {
			a := int32(2 * (i - 1))
			triangles = append(triangles, a, a+1, a+2, a+1, a+3, a+2)
		}
	}
	freedom := make([]float32, len(vertices))
	for i := range freedom {
		freedom[i] = 0.08
	}
	c := newClothMesh(vertices, triangles, freedom)
	copy(c.target, vertices)
	for p := range c.lastTarget {
		c.lastTarget[p] = rl.Vector3Add(c.target[p], rl.Vector3{Z: 0.03}) // it moved this frame
	}
	colliders := []worldCapsule{
		{a: rl.Vector3{X: -0.3, Y: 0.2, Z: 0.02}, b: rl.Vector3{X: 0.3, Y: 0.25, Z: 0.02}, radius: 0.06},
		{a: rl.Vector3{Y: 0.5, Z: -0.05}, b: rl.Vector3{Y: 0.9, Z: 0.05}, radius: 0.07},
		{a: rl.Vector3{X: 2, Y: 0.5}, b: rl.Vector3{X: 2, Y: 0.9}, radius: 0.1}, // out of reach
	}
	c.reach(colliders)
	if len(c.near) == 0 || len(c.near) >= len(c.free)*len(colliders) {
		t.Fatalf("reach kept %d of %d pairs, want some but not all", len(c.near), len(c.free)*len(colliders))
	}
	for i, p := range c.free {
		for _, off := range []rl.Vector3{{}, {Z: 0.08}, {Z: -0.08}, {X: 0.05, Y: -0.05}, {X: -0.04, Z: 0.06}} {
			for _, target := range []rl.Vector3{c.target[p], c.lastTarget[p]} {
				x := rl.Vector3Add(target, off)
				if got, want := c.collide(i, x, target, colliders), collide(x, target, colliders); got != want {
					t.Fatalf("particle %d at %v: %v by the colliders in reach, %v by them all", p, x, got, want)
				}
			}
		}
	}
}
