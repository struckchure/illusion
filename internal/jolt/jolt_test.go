package jolt

import (
	"math"
	"testing"
)

func newTestWorld(t *testing.T) (*World, BodyID) {
	t.Helper()
	w := NewWorld(1024)
	t.Cleanup(w.Close)
	w.SetGravity([3]float32{0, -9.81, 0})

	ground, err := NewBox([3]float32{50, 0.5, 50})
	if err != nil {
		t.Fatal(err)
	}
	defer ground.Release()
	s := DefaultBodySettings(ground)
	s.Motion = Static
	s.Transform.Position = [3]float32{0, -0.5, 0}
	return w, w.CreateBody(s)
}

func step(w *World, seconds float32) {
	for range int(seconds * 60) {
		w.Step(1.0/60, 1)
	}
}

func TestFallingBoxLandsAndReportsContact(t *testing.T) {
	w, ground := newTestWorld(t)
	box, _ := NewBox([3]float32{0.5, 0.5, 0.5})
	defer box.Release()
	s := DefaultBodySettings(box)
	s.Transform.Position = [3]float32{0, 5, 0}
	id := w.CreateBody(s)

	step(w, 3)
	y := w.Transform(id).Position[1]
	if math.Abs(float64(y-0.5)) > 0.05 {
		t.Fatalf("box should rest on the ground at y=0.5, got %v", y)
	}

	contacts := w.DrainContacts(nil)
	found := false
	for _, c := range contacts {
		pair := (c.Body1 == id && c.Body2 == ground) || (c.Body1 == ground && c.Body2 == id)
		if pair && c.Began {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a contact between box and ground, got %+v", contacts)
	}
}

func TestTiltedBoxTopplesAndRotates(t *testing.T) {
	w, _ := newTestWorld(t)
	box, _ := NewBox([3]float32{0.5, 0.5, 0.5})
	defer box.Release()
	s := DefaultBodySettings(box)
	// Balanced on an edge, tipped slightly: it should fall over and turn.
	half := float32(math.Sin(math.Pi / 8))
	s.Transform = Transform{Position: [3]float32{0, 0.75, 0}, Rotation: [4]float32{0, 0, half * 1.1, float32(math.Cos(math.Pi / 8))}}
	id := w.CreateBody(s)

	step(w, 3)
	q := w.Transform(id).Rotation
	if math.Abs(float64(q[2])) < 0.1 && math.Abs(float64(q[3])) > 0.99 {
		t.Fatalf("box should have rotated, got quaternion %v", q)
	}
}

func TestVelocityImpulseAndDestroy(t *testing.T) {
	w, _ := newTestWorld(t)
	sphere, _ := NewSphere(0.5)
	defer sphere.Release()
	s := DefaultBodySettings(sphere)
	s.Transform.Position = [3]float32{0, 0.5, 0}
	s.GravityFactor = 0
	s.LinearDamping = 0
	id := w.CreateBody(s)

	w.SetVelocity(id, [3]float32{2, 0, 0}, [3]float32{})
	w.Step(1, 1)
	if x := w.Transform(id).Position[0]; math.Abs(float64(x-2)) > 0.2 {
		t.Fatalf("expected x≈2 after 1s at 2m/s, got %v", x)
	}
	w.AddImpulse(id, [3]float32{0, 10, 0})
	if lin, _ := w.Velocity(id); lin[1] <= 0 {
		t.Fatalf("impulse should add upward velocity, got %v", lin)
	}

	w.DestroyBody(id)
	if _, ok := w.CastRay([3]float32{-5, 0.5, 0}, [3]float32{20, 0, 0}, InvalidBody); ok {
		t.Fatal("destroyed body should not be hit")
	}
}

func TestRaycast(t *testing.T) {
	w, ground := newTestWorld(t)
	hit, ok := w.CastRay([3]float32{0, 10, 0}, [3]float32{0, -20, 0}, InvalidBody)
	if !ok || hit.Body != ground {
		t.Fatalf("ray should hit the ground, got %+v ok=%v", hit, ok)
	}
	if math.Abs(float64(hit.Point[1])) > 1e-3 || hit.Normal[1] < 0.99 {
		t.Fatalf("hit at %v normal %v", hit.Point, hit.Normal)
	}
	if _, ok := w.CastRay([3]float32{0, 10, 0}, [3]float32{0, -20, 0}, ground); ok {
		t.Fatal("ignored body should not be hit")
	}
}

func TestCharacterWalksAndStands(t *testing.T) {
	w, _ := newTestWorld(t)
	capsule, _ := NewCapsule(0.5, 0.3)
	defer capsule.Release()
	c := w.NewCharacter(capsule, [3]float32{0, 3, 0}, math.Pi/4, 0.3)
	defer c.Close()

	g := w.Gravity()
	for range 180 {
		v := c.Velocity()
		if c.Supported() {
			v = [3]float32{1, 0, 0}
		} else {
			v[1] += g[1] / 60
		}
		c.SetVelocity(v)
		c.Update(1.0/60, 0.3)
		w.Step(1.0/60, 1)
	}
	p := c.Position()
	if !c.Supported() {
		t.Fatalf("character should be standing, at %v", p)
	}
	// Center of a 0.5+0.3 half-height capsule standing on y=0.
	if math.Abs(float64(p[1]-0.8)) > 0.05 || p[0] < 0.5 {
		t.Fatalf("character at %v", p)
	}
	if c.InnerBody() == InvalidBody {
		t.Fatal("character should have an inner body")
	}
}

func TestSensorDetectsCharacter(t *testing.T) {
	w, _ := newTestWorld(t)
	box, _ := NewBox([3]float32{1, 1, 1})
	defer box.Release()
	s := DefaultBodySettings(box)
	s.Motion = Static
	s.Sensor = true
	s.Transform.Position = [3]float32{0, 1, 0}
	sensor := w.CreateBody(s)

	capsule, _ := NewCapsule(0.5, 0.3)
	defer capsule.Release()
	c := w.NewCharacter(capsule, [3]float32{0, 0.8, 5}, math.Pi/4, 0.3)
	defer c.Close()

	var contacts []Contact
	for range 120 {
		c.SetVelocity([3]float32{0, 0, -5})
		c.Update(1.0/60, 0)
		w.Step(1.0/60, 1)
		contacts = w.DrainContacts(contacts)
	}
	began := false
	for _, ct := range contacts {
		if ct.Began && (ct.Body1 == sensor || ct.Body2 == sensor) {
			began = true
		}
	}
	if !began {
		t.Fatalf("sensor should detect the character walking through it; got %+v", contacts)
	}
}

func TestCharacterReportsStaticContacts(t *testing.T) {
	w, _ := newTestWorld(t)
	post, _ := NewCylinder(1, 0.25)
	defer post.Release()
	s := DefaultBodySettings(post)
	s.Motion = Static
	s.Transform.Position = [3]float32{2, 1, 0}
	pole := w.CreateBody(s)

	capsule, _ := NewCapsule(0.5, 0.3)
	defer capsule.Release()
	c := w.NewCharacter(capsule, [3]float32{0, 0.8, 0}, math.Pi/4, 0.3)
	defer c.Close()

	var contacts []Contact
	for range 60 {
		c.SetVelocity([3]float32{3, 0, 0})
		c.Update(1.0/60, 0)
		w.Step(1.0/60, 1)
		contacts = w.DrainContacts(contacts)
	}
	for _, ct := range contacts {
		if ct.Began && ct.Body1 == c.InnerBody() && ct.Body2 == pole {
			if ct.Normal[0] < 0.9 {
				t.Fatalf("normal should point from character to pole, got %v", ct.Normal)
			}
			return
		}
	}
	t.Fatalf("expected character/pole contact, got %+v (character %v)", contacts, c.InnerBody())
}

// gridMesh is a flat n x n grid of 1m quads (two triangles each) centered on
// the origin at y=0.
func gridMesh(t *testing.T, n int) *Shape {
	t.Helper()
	var verts [][3]float32
	var idx []uint32
	half := float32(n) / 2
	for z := 0; z <= n; z++ {
		for x := 0; x <= n; x++ {
			verts = append(verts, [3]float32{float32(x) - half, 0, float32(z) - half})
		}
	}
	for z := 0; z < n; z++ {
		for x := 0; x < n; x++ {
			a := uint32(z*(n+1) + x)
			b, c, d := a+1, a+uint32(n+1), a+uint32(n+2)
			idx = append(idx, a, c, b, b, c, d)
		}
	}
	s, err := NewMesh(verts, idx)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestContactsAreReportedOncePerBodyPair(t *testing.T) {
	w := NewWorld(1024)
	t.Cleanup(w.Close)
	w.SetGravity([3]float32{0, -9.81, 0})
	mesh := gridMesh(t, 20)
	defer mesh.Release()
	gs := DefaultBodySettings(mesh)
	gs.Motion = Static
	ground := w.CreateBody(gs)

	// A box landing on a grid vertex touches several triangles at once, then
	// slides across more of them.
	box, _ := NewBox([3]float32{0.5, 0.5, 0.5})
	defer box.Release()
	s := DefaultBodySettings(box)
	s.Transform.Position = [3]float32{0, 2, 0}
	s.LinearVelocity = [3]float32{2, 0, 1}
	s.Friction = 0.05
	id := w.CreateBody(s)

	var contacts []Contact
	for range 120 {
		w.Step(1.0/60, 1)
		contacts = w.DrainContacts(contacts)
	}
	begins, ends := 0, 0
	for _, c := range contacts {
		if (c.Body1 == id && c.Body2 == ground) || (c.Body1 == ground && c.Body2 == id) {
			if c.Began {
				begins++
			} else {
				ends++
			}
		}
	}
	if begins != 1 || ends != 0 {
		t.Fatalf("a box resting and sliding on a mesh should begin one contact and end none, got %d begins, %d ends", begins, ends)
	}
}

func TestDestroyingABodyEndsItsContacts(t *testing.T) {
	w, ground := newTestWorld(t)
	box, _ := NewBox([3]float32{0.5, 0.5, 0.5})
	defer box.Release()
	s := DefaultBodySettings(box)
	s.Transform.Position = [3]float32{0, 0.5, 0}
	id := w.CreateBody(s)
	step(w, 0.5)
	w.DrainContacts(nil)

	w.DestroyBody(id)
	ended := false
	for _, c := range w.DrainContacts(nil) {
		if !c.Began && ((c.Body1 == id && c.Body2 == ground) || (c.Body1 == ground && c.Body2 == id)) {
			ended = true
		}
	}
	if !ended {
		t.Fatal("destroying a resting box should end its contact with the ground")
	}
	step(w, 0.5) // Jolt's own removal report for the pair must not repeat it
	for _, c := range w.DrainContacts(nil) {
		if c.Body1 == id || c.Body2 == id {
			t.Fatalf("unexpected contact for the destroyed body: %+v", c)
		}
	}
}

func TestDynamicSensorFalls(t *testing.T) {
	w, _ := newTestWorld(t)
	ball, _ := NewSphere(0.5)
	defer ball.Release()
	s := DefaultBodySettings(ball)
	s.Sensor = true
	s.Transform.Position = [3]float32{0, 3, 0}
	id := w.CreateBody(s)
	step(w, 1)
	if y := w.Transform(id).Position[1]; y > -1 {
		t.Fatalf("a dynamic sensor should fall through the ground, still at y=%v", y)
	}
}

func TestSleepingKeepsContactsAndWakingAwayEndsThem(t *testing.T) {
	w, ground := newTestWorld(t)
	box, _ := NewBox([3]float32{0.5, 0.5, 0.5})
	defer box.Release()
	s := DefaultBodySettings(box)
	s.Transform.Position = [3]float32{0, 0.5, 0}
	id := w.CreateBody(s)

	pairEvents := func(cs []Contact) (begins, ends int) {
		for _, c := range cs {
			if (c.Body1 == id && c.Body2 == ground) || (c.Body1 == ground && c.Body2 == id) {
				if c.Began {
					begins++
				} else {
					ends++
				}
			}
		}
		return begins, ends
	}

	step(w, 3)
	if w.IsActive(id) {
		t.Fatal("the box should have fallen asleep")
	}
	if b, e := pairEvents(w.DrainContacts(nil)); b != 1 || e != 0 {
		t.Fatalf("falling asleep must not end the contact: %d begins, %d ends", b, e)
	}

	w.Activate(id) // woken in place: still touching
	step(w, 0.2)
	if b, e := pairEvents(w.DrainContacts(nil)); b != 0 || e != 0 {
		t.Fatalf("waking in place must not report anything: %d begins, %d ends", b, e)
	}

	step(w, 3) // asleep again
	w.SetTransform(id, Transform{Position: [3]float32{0, 10, 0}, Rotation: [4]float32{0, 0, 0, 1}}, true)
	step(w, 0.1)
	if b, e := pairEvents(w.DrainContacts(nil)); b != 0 || e != 1 {
		t.Fatalf("waking away from the ground should end the contact once: %d begins, %d ends", b, e)
	}
}
