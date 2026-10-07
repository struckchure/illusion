//go:build js

// The browser binding: glue.cpp and Jolt run as their own emscripten module
// (built by web/lib.sh), which the page loads into globalThis.jolt before
// starting Go. Pointers into that module are uint32 addresses.

package jolt

import (
	"unsafe"

	"github.com/struckchure/illusion/internal/emscripten"
)

// C structs from glue.h, laid out as in wasm32. glue.cpp asserts the sizes.
type (
	cBodySettings struct {
		Shape           uint32
		Transform       [7]float32
		LinearVelocity  [3]float32
		AngularVelocity [3]float32
		Motion          int32
		Sensor          int32
		AllowSleeping   int32
		Continuous      int32
		AllowedDOFs     uint32
		Friction        float32
		Restitution     float32
		LinearDamping   float32
		AngularDamping  float32
		GravityFactor   float32
		Mass            float32
		Character       int32
		UserData        uint64
	}
	cContactEvent struct {
		Body1, Body2 uint32
		Kind         int32
		Point        [3]float32
		Normal       [3]float32
	}
	cRayHit struct {
		Body     uint32
		Fraction float32
		Point    [3]float32
		Normal   [3]float32
	}
)

const _ = uint(unsafe.Sizeof(cBodySettings{})-112) + uint(112-unsafe.Sizeof(cBodySettings{})) +
	uint(unsafe.Sizeof(cContactEvent{})-36) + uint(36-unsafe.Sizeof(cContactEvent{})) +
	uint(unsafe.Sizeof(cRayHit{})-32) + uint(32-unsafe.Sizeof(cRayHit{}))

const contactBegin = 0 // ILL_CONTACT_BEGIN

var module *emscripten.Module

// mod loads the module on first use, so pages without physics needn't ship it.
func mod() *emscripten.Module {
	if module == nil {
		module = emscripten.Load("jolt")
		module.Call("ILL_Init")
	}
	return module
}

// World is a physics simulation. It is not safe for concurrent use.
type World struct {
	w        uint32
	contacts []cContactEvent
}

// NewWorld creates a world that can hold up to maxBodies bodies.
func NewWorld(maxBodies int) *World {
	return &World{
		w:        emscripten.Uint32(mod().Call("ILL_World_New", maxBodies)),
		contacts: make([]cContactEvent, 256),
	}
}

// Close frees the world and every body in it.
func (w *World) Close() {
	if w.w != 0 {
		mod().Call("ILL_World_Delete", w.w)
		w.w = 0
	}
}

// Step advances the simulation by dt seconds, split into collisionSteps
// sub-steps.
func (w *World) Step(dt float32, collisionSteps int) error {
	if mod().Call("ILL_World_Step", w.w, dt, collisionSteps).Int() != 0 {
		return errOverflow
	}
	return nil
}

// SetGravity sets the acceleration applied to dynamic bodies.
func (w *World) SetGravity(g [3]float32) {
	m := mod()
	m.Call("ILL_World_SetGravity", w.w, emscripten.Arg(m, g))
}

// Gravity returns the world's gravity.
func (w *World) Gravity() [3]float32 {
	m := mod()
	out := m.Alloc(12)
	m.Call("ILL_World_GetGravity", w.w, out)
	return emscripten.Read[[3]float32](m, out)
}

// Shape is collision geometry. Bodies keep their own reference, so release a
// shape once the bodies using it are created.
type Shape struct {
	s uint32
}

func shapeOrErr(p uint32) (*Shape, error) {
	if p == 0 {
		return nil, errShape
	}
	return &Shape{s: p}, nil
}

// NewBox makes a box with the given half extents.
func NewBox(halfExtents [3]float32) (*Shape, error) {
	m := mod()
	return shapeOrErr(emscripten.Uint32(m.Call("ILL_Shape_Box", emscripten.Arg(m, halfExtents))))
}

// NewSphere makes a sphere.
func NewSphere(radius float32) (*Shape, error) {
	return shapeOrErr(emscripten.Uint32(mod().Call("ILL_Shape_Sphere", radius)))
}

// NewCapsule makes a Y-aligned capsule; halfHeight is half the straight part.
func NewCapsule(halfHeight, radius float32) (*Shape, error) {
	return shapeOrErr(emscripten.Uint32(mod().Call("ILL_Shape_Capsule", halfHeight, radius)))
}

// NewCylinder makes a Y-aligned cylinder centered on the origin.
func NewCylinder(halfHeight, radius float32) (*Shape, error) {
	return shapeOrErr(emscripten.Uint32(mod().Call("ILL_Shape_Cylinder", halfHeight, radius)))
}

// NewConvexHull makes the convex hull of points.
func NewConvexHull(points [][3]float32) (*Shape, error) {
	if len(points) == 0 {
		return nil, errShape
	}
	m := mod()
	return shapeOrErr(emscripten.Uint32(m.Call("ILL_Shape_ConvexHull", emscripten.Slice(m, points), len(points))))
}

// NewMesh makes a triangle mesh, for static geometry. indices holds three
// vertex indices per triangle.
func NewMesh(vertices [][3]float32, indices []uint32) (*Shape, error) {
	if len(vertices) == 0 || len(indices) < 3 {
		return nil, errShape
	}
	m := mod()
	return shapeOrErr(emscripten.Uint32(m.Call("ILL_Shape_Mesh",
		emscripten.Slice(m, vertices), len(vertices), emscripten.Slice(m, indices), len(indices)/3)))
}

// Offset returns a new shape that is s moved by offset. s is not consumed.
func (s *Shape) Offset(offset [3]float32) (*Shape, error) {
	m := mod()
	return shapeOrErr(emscripten.Uint32(m.Call("ILL_Shape_Offset", s.s, emscripten.Arg(m, offset))))
}

// Release drops this reference to the shape.
func (s *Shape) Release() {
	if s != nil && s.s != 0 {
		mod().Call("ILL_Shape_Release", s.s)
		s.s = 0
	}
}

func defaultSettings() cBodySettings {
	m := mod()
	out := m.Alloc(int(unsafe.Sizeof(cBodySettings{})))
	m.Call("ILL_BodySettings_Default", out)
	return emscripten.Read[cBodySettings](m, out)
}

// DefaultBodySettings is a dynamic body at the origin with Jolt's defaults.
func DefaultBodySettings(shape *Shape) BodySettings {
	c := defaultSettings()
	return BodySettings{
		Shape:          shape,
		Transform:      Transform{Rotation: [4]float32{0, 0, 0, 1}},
		Motion:         int(c.Motion),
		AllowSleeping:  c.AllowSleeping != 0,
		AllowedDOFs:    c.AllowedDOFs,
		Friction:       c.Friction,
		Restitution:    c.Restitution,
		LinearDamping:  c.LinearDamping,
		AngularDamping: c.AngularDamping,
		GravityFactor:  c.GravityFactor,
	}
}

func cbool(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// CreateBody adds a body to the world.
func (w *World) CreateBody(s BodySettings) BodyID {
	c := defaultSettings()
	c.Shape = s.Shape.s
	c.Transform = s.Transform.array()
	c.LinearVelocity = s.LinearVelocity
	c.AngularVelocity = s.AngularVelocity
	c.Motion = int32(s.Motion)
	c.Sensor = cbool(s.Sensor)
	c.AllowSleeping = cbool(s.AllowSleeping)
	c.Continuous = cbool(s.Continuous)
	c.AllowedDOFs = s.AllowedDOFs
	c.Friction = s.Friction
	c.Restitution = s.Restitution
	c.LinearDamping = s.LinearDamping
	c.AngularDamping = s.AngularDamping
	c.GravityFactor = s.GravityFactor
	c.Mass = s.Mass
	c.Character = cbool(s.Character)
	m := mod()
	return BodyID(emscripten.Uint32(m.Call("ILL_Body_Create", w.w, emscripten.Arg(m, c))))
}

// DestroyBody removes a body from the world and frees it.
func (w *World) DestroyBody(id BodyID) { mod().Call("ILL_Body_Destroy", w.w, uint32(id)) }

// Transform returns a body's position and rotation.
func (w *World) Transform(id BodyID) Transform {
	m := mod()
	out := m.Alloc(28)
	m.Call("ILL_Body_GetTransform", w.w, uint32(id), out)
	return transformOf(emscripten.Read[[7]float32](m, out))
}

// SetTransform teleports a body.
func (w *World) SetTransform(id BodyID, t Transform, activate bool) {
	m := mod()
	m.Call("ILL_Body_SetTransform", w.w, uint32(id), emscripten.Arg(m, t.array()), emscripten.Int(activate))
}

// MoveKinematic moves a kinematic body to t over dt seconds, so it pushes
// dynamic bodies along the way.
func (w *World) MoveKinematic(id BodyID, t Transform, dt float32) {
	m := mod()
	m.Call("ILL_Body_MoveKinematic", w.w, uint32(id), emscripten.Arg(m, t.array()), dt)
}

// Velocity returns a body's linear and angular velocity.
func (w *World) Velocity(id BodyID) (linear, angular [3]float32) {
	m := mod()
	out := m.Alloc(24)
	m.Call("ILL_Body_GetVelocity", w.w, uint32(id), out)
	a := emscripten.Read[[6]float32](m, out)
	return [3]float32{a[0], a[1], a[2]}, [3]float32{a[3], a[4], a[5]}
}

// SetVelocity sets a body's linear and angular velocity and wakes it.
func (w *World) SetVelocity(id BodyID, linear, angular [3]float32) {
	m := mod()
	a := [6]float32{linear[0], linear[1], linear[2], angular[0], angular[1], angular[2]}
	m.Call("ILL_Body_SetVelocity", w.w, uint32(id), emscripten.Arg(m, a))
}

func (w *World) bodyVec(fn string, id BodyID, v [3]float32) {
	m := mod()
	m.Call(fn, w.w, uint32(id), emscripten.Arg(m, v))
}

// AddForce applies a force (Newtons) at the center of mass for the next step.
func (w *World) AddForce(id BodyID, f [3]float32) { w.bodyVec("ILL_Body_AddForce", id, f) }

// AddTorque applies a torque for the next step.
func (w *World) AddTorque(id BodyID, t [3]float32) { w.bodyVec("ILL_Body_AddTorque", id, t) }

// AddImpulse changes a body's momentum immediately.
func (w *World) AddImpulse(id BodyID, i [3]float32) { w.bodyVec("ILL_Body_AddImpulse", id, i) }

// AddAngularImpulse changes a body's angular momentum immediately.
func (w *World) AddAngularImpulse(id BodyID, i [3]float32) {
	w.bodyVec("ILL_Body_AddAngularImpulse", id, i)
}

// IsActive reports whether a body is awake.
func (w *World) IsActive(id BodyID) bool {
	return emscripten.Bool(mod().Call("ILL_Body_IsActive", w.w, uint32(id)))
}

// Activate wakes a body.
func (w *World) Activate(id BodyID) { mod().Call("ILL_Body_Activate", w.w, uint32(id)) }

// DrainContacts appends the contacts since the last call to buf.
func (w *World) DrainContacts(buf []Contact) []Contact {
	m := mod()
	for {
		out := m.Alloc(len(w.contacts) * int(unsafe.Sizeof(cContactEvent{})))
		n := m.Call("ILL_World_DrainContacts", w.w, out, len(w.contacts)).Int()
		emscripten.ReadSlice(m, out, w.contacts[:n])
		for _, c := range w.contacts[:n] {
			buf = append(buf, Contact{
				Body1:  BodyID(c.Body1),
				Body2:  BodyID(c.Body2),
				Began:  c.Kind == contactBegin,
				Point:  c.Point,
				Normal: c.Normal,
			})
		}
		if n < len(w.contacts) {
			return buf
		}
	}
}

// CastRay casts from origin along dir (whose length is the maximum distance),
// skipping the body ignore (InvalidBody skips nothing).
func (w *World) CastRay(origin, dir [3]float32, ignore BodyID) (RayHit, bool) {
	m := mod()
	out := m.Alloc(int(unsafe.Sizeof(cRayHit{})))
	if !emscripten.Bool(m.Call("ILL_World_CastRay", w.w, emscripten.Arg(m, origin), emscripten.Arg(m, dir), uint32(ignore), out)) {
		return RayHit{}, false
	}
	h := emscripten.Read[cRayHit](m, out)
	return RayHit{Body: BodyID(h.Body), Fraction: h.Fraction, Point: h.Point, Normal: h.Normal}, true
}

// Character is a kinematic character controller: it slides along walls,
// walks up slopes and stairs, and stands on moving platforms.
type Character struct {
	c uint32
}

// NewCharacter creates a character. shape should be a capsule whose bottom
// sphere has radius supportRadius; contacts below its center count as ground.
func (w *World) NewCharacter(shape *Shape, position [3]float32, maxSlope, supportRadius float32) *Character {
	m := mod()
	return &Character{c: emscripten.Uint32(m.Call("ILL_Character_New", w.w, shape.s, emscripten.Arg(m, position), maxSlope, supportRadius))}
}

// Close removes the character from the world.
func (c *Character) Close() {
	if c.c != 0 {
		mod().Call("ILL_Character_Delete", c.c)
		c.c = 0
	}
}

// Update moves the character by its velocity over dt, climbing steps up to
// stepUp high.
func (c *Character) Update(dt, stepUp float32) { mod().Call("ILL_Character_Update", c.c, dt, stepUp) }

func (c *Character) getVec(fn string) [3]float32 {
	m := mod()
	out := m.Alloc(12)
	m.Call(fn, c.c, out)
	return emscripten.Read[[3]float32](m, out)
}

func (c *Character) setVec(fn string, v [3]float32) {
	m := mod()
	m.Call(fn, c.c, emscripten.Arg(m, v))
}

// Position returns the character's position.
func (c *Character) Position() [3]float32 { return c.getVec("ILL_Character_GetPosition") }

// SetPosition teleports the character.
func (c *Character) SetPosition(p [3]float32) { c.setVec("ILL_Character_SetPosition", p) }

// Velocity returns the character's velocity.
func (c *Character) Velocity() [3]float32 { return c.getVec("ILL_Character_GetVelocity") }

// SetVelocity sets the velocity the next Update moves by.
func (c *Character) SetVelocity(v [3]float32) { c.setVec("ILL_Character_SetVelocity", v) }

// GroundVelocity is the velocity of whatever the character stands on.
func (c *Character) GroundVelocity() [3]float32 { return c.getVec("ILL_Character_GetGroundVelocity") }

// GroundNormal is the surface normal of whatever the character stands on.
func (c *Character) GroundNormal() [3]float32 { return c.getVec("ILL_Character_GetGroundNormal") }

// Supported reports whether the character stands on something walkable.
func (c *Character) Supported() bool {
	return emscripten.Bool(mod().Call("ILL_Character_IsSupported", c.c))
}

// InnerBody is the rigid body that represents the character to other bodies.
func (c *Character) InnerBody() BodyID {
	return BodyID(emscripten.Uint32(mod().Call("ILL_Character_InnerBody", c.c)))
}

func (w *World) OverlapCapsule(center [3]float32, radius, height float32, ignore BodyID) bool {
	m := mod()
	return emscripten.Bool(m.Call("ILL_World_OverlapCapsule", w.w, emscripten.Arg(m, center), radius, height, uint32(ignore)))
}
func (w *World) SweepCapsule(center, delta [3]float32, radius, height float32, ignore BodyID) (RayHit, bool) {
	m := mod()
	out := m.Alloc(int(unsafe.Sizeof(cRayHit{})))
	if !emscripten.Bool(m.Call("ILL_World_SweepCapsule", w.w, emscripten.Arg(m, center), emscripten.Arg(m, delta), radius, height, uint32(ignore), out)) {
		return RayHit{}, false
	}
	h := emscripten.Read[cRayHit](m, out)
	return RayHit{Body: BodyID(h.Body), Fraction: h.Fraction, Point: h.Point, Normal: h.Normal}, true
}
func (c *Character) UpdateControlled(dt float32) {
	mod().Call("ILL_Character_UpdateControlled", c.c, dt)
}

// The vehicle structs in types.go are laid out as in wasm32; glue.cpp asserts
// the sizes.
const _ = uint(unsafe.Sizeof(WheelSettings{})-144) + uint(144-unsafe.Sizeof(WheelSettings{})) +
	uint(unsafe.Sizeof(VehicleSettings{})-128) + uint(128-unsafe.Sizeof(VehicleSettings{})) +
	uint(unsafe.Sizeof(Differential{})-24) + uint(24-unsafe.Sizeof(Differential{})) +
	uint(unsafe.Sizeof(AntiRollBar{})-12) + uint(12-unsafe.Sizeof(AntiRollBar{}))

// OffsetCenterOfMass returns a new shape that is s with its center of mass
// moved by offset. s is not consumed.
func (s *Shape) OffsetCenterOfMass(offset [3]float32) (*Shape, error) {
	m := mod()
	return shapeOrErr(emscripten.Uint32(m.Call("ILL_Shape_OffsetCenterOfMass", s.s, emscripten.Arg(m, offset))))
}

// CenterOfMass is where the shape's center of mass is, in its own space.
func (s *Shape) CenterOfMass() [3]float32 {
	m := mod()
	out := m.Alloc(12)
	m.Call("ILL_Shape_GetCenterOfMass", s.s, out)
	return emscripten.Read[[3]float32](m, out)
}

func readDefault[T any](fn string) T {
	var v T
	m := mod()
	out := m.Alloc(int(unsafe.Sizeof(v)))
	m.Call(fn, out)
	return emscripten.Read[T](m, out)
}

// DefaultWheelSettings is Jolt's default wheel.
func DefaultWheelSettings() WheelSettings { return readDefault[WheelSettings]("ILL_WheelDesc_Default") }

// DefaultVehicleSettings is Jolt's default car: +Y up, +Z forward.
func DefaultVehicleSettings() VehicleSettings {
	return readDefault[VehicleSettings]("ILL_VehicleDesc_Default")
}

// DefaultDifferential is Jolt's default differential, with no wheels.
func DefaultDifferential() Differential { return readDefault[Differential]("ILL_Differential_Default") }

// CreateVehicle makes a dynamic body a vehicle. It reports false if the body
// isn't dynamic, already is one, or wheels is empty.
func (w *World) CreateVehicle(body BodyID, s VehicleSettings, wheels []WheelSettings, diffs []Differential, bars []AntiRollBar) bool {
	if len(wheels) == 0 {
		return false
	}
	m := mod()
	return emscripten.Bool(m.Call("ILL_Vehicle_Create", w.w, uint32(body), emscripten.Arg(m, s),
		emscripten.Slice(m, wheels), len(wheels), emscripten.Slice(m, diffs), len(diffs), emscripten.Slice(m, bars), len(bars)))
}

// DestroyVehicle takes a body's vehicle away, leaving the body. Destroying the
// body does this too.
func (w *World) DestroyVehicle(body BodyID) { mod().Call("ILL_Vehicle_Destroy", w.w, uint32(body)) }

// SetVehicleInput sets what the driver does until it's next set: forward and
// right from -1 to 1, brake and handBrake from 0 to 1.
func (w *World) SetVehicleInput(body BodyID, forward, right, brake, handBrake float32) {
	mod().Call("ILL_Vehicle_SetInput", w.w, uint32(body), forward, right, brake, handBrake)
}

// VehicleWheels appends a vehicle's wheels to buf.
func (w *World) VehicleWheels(body BodyID, buf []WheelState) []WheelState {
	m := mod()
	const most = 16
	out := m.Alloc(most * wheelStateFloats * 4)
	n := m.Call("ILL_Vehicle_GetWheels", w.w, uint32(body), out, most).Int()
	if n > most {
		out = m.Alloc(n * wheelStateFloats * 4)
		m.Call("ILL_Vehicle_GetWheels", w.w, uint32(body), out, n)
	}
	a := make([]float32, n*wheelStateFloats)
	emscripten.ReadSlice(m, out, a)
	return wheelStates(buf, a)
}

// VehicleStatus returns a vehicle's engine and motion.
func (w *World) VehicleStatus(body BodyID) VehicleStatus {
	m := mod()
	out := m.Alloc(16)
	m.Call("ILL_Vehicle_GetStatus", w.w, uint32(body), out)
	return vehicleStatus(emscripten.Read[[4]float32](m, out))
}
