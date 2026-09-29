// Package jolt is a thin cgo binding over Jolt Physics. Jolt is vendored in
// third_party and compiled from source (see update.sh); glue.cpp exposes the
// small C API declared in glue.h.
//
// Vectors are [3]float32 and transforms are position plus quaternion (x, y, z,
// w), so callers can convert from their own math types without copying
// through C structs.
package jolt

/*
#cgo CXXFLAGS: -std=c++17 -O2 -DNDEBUG -I${SRCDIR}/third_party/JoltPhysics -Wno-unused-parameter
#cgo darwin LDFLAGS: -lc++
#cgo linux LDFLAGS: -lstdc++ -lm -lpthread
#include <stdlib.h>
#include "glue.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Motion types.
const (
	Static    = C.ILL_STATIC
	Kinematic = C.ILL_KINEMATIC
	Dynamic   = C.ILL_DYNAMIC
)

// Degrees of freedom for BodySettings.AllowedDOFs.
const (
	TranslationX = 1 << iota
	TranslationY
	TranslationZ
	RotationX
	RotationY
	RotationZ
	AllDOFs = TranslationX | TranslationY | TranslationZ | RotationX | RotationY | RotationZ
)

// BodyID identifies a body in a World.
type BodyID uint32

// InvalidBody is never a real body.
const InvalidBody BodyID = C.ILL_INVALID_BODY

// Transform is a position and a rotation quaternion.
type Transform struct {
	Position [3]float32
	Rotation [4]float32 // x, y, z, w
}

func (t *Transform) array() [7]float32 {
	return [7]float32{t.Position[0], t.Position[1], t.Position[2], t.Rotation[0], t.Rotation[1], t.Rotation[2], t.Rotation[3]}
}

func transformOf(a [7]float32) Transform {
	return Transform{Position: [3]float32{a[0], a[1], a[2]}, Rotation: [4]float32{a[3], a[4], a[5], a[6]}}
}

func fp(p *float32) *C.float { return (*C.float)(unsafe.Pointer(p)) }

// World is a physics simulation. It is not safe for concurrent use.
type World struct {
	w        *C.ILL_World
	contacts []C.ILL_ContactEvent
}

// NewWorld creates a world that can hold up to maxBodies bodies.
func NewWorld(maxBodies int) *World {
	return &World{
		w:        C.ILL_World_New(C.uint32_t(maxBodies)),
		contacts: make([]C.ILL_ContactEvent, 256),
	}
}

// Close frees the world and every body in it.
func (w *World) Close() {
	if w.w != nil {
		C.ILL_World_Delete(w.w)
		w.w = nil
	}
}

// Step advances the simulation by dt seconds, split into collisionSteps
// sub-steps.
func (w *World) Step(dt float32, collisionSteps int) error {
	if code := C.ILL_World_Step(w.w, C.float(dt), C.int(collisionSteps)); code != 0 {
		return errors.New("jolt: physics update overflowed its buffers; raise the world's body limit")
	}
	return nil
}

// SetGravity sets the acceleration applied to dynamic bodies.
func (w *World) SetGravity(g [3]float32) { C.ILL_World_SetGravity(w.w, fp(&g[0])) }

// Gravity returns the world's gravity.
func (w *World) Gravity() (g [3]float32) {
	C.ILL_World_GetGravity(w.w, fp(&g[0]))
	return g
}

// Shape is collision geometry. Bodies keep their own reference, so release a
// shape once the bodies using it are created.
type Shape struct {
	s *C.ILL_Shape
}

var errShape = errors.New("jolt: invalid shape")

func shapeOrErr(s *C.ILL_Shape) (*Shape, error) {
	if s == nil {
		return nil, errShape
	}
	return &Shape{s: s}, nil
}

// NewBox makes a box with the given half extents.
func NewBox(halfExtents [3]float32) (*Shape, error) {
	return shapeOrErr(C.ILL_Shape_Box(fp(&halfExtents[0])))
}

// NewSphere makes a sphere.
func NewSphere(radius float32) (*Shape, error) {
	return shapeOrErr(C.ILL_Shape_Sphere(C.float(radius)))
}

// NewCapsule makes a Y-aligned capsule; halfHeight is half the straight part.
func NewCapsule(halfHeight, radius float32) (*Shape, error) {
	return shapeOrErr(C.ILL_Shape_Capsule(C.float(halfHeight), C.float(radius)))
}

// NewCylinder makes a Y-aligned cylinder centered on the origin.
func NewCylinder(halfHeight, radius float32) (*Shape, error) {
	return shapeOrErr(C.ILL_Shape_Cylinder(C.float(halfHeight), C.float(radius)))
}

// NewConvexHull makes the convex hull of points.
func NewConvexHull(points [][3]float32) (*Shape, error) {
	if len(points) == 0 {
		return nil, errShape
	}
	return shapeOrErr(C.ILL_Shape_ConvexHull(fp(&points[0][0]), C.int(len(points))))
}

// NewMesh makes a triangle mesh, for static geometry. indices holds three
// vertex indices per triangle.
func NewMesh(vertices [][3]float32, indices []uint32) (*Shape, error) {
	if len(vertices) == 0 || len(indices) < 3 {
		return nil, errShape
	}
	return shapeOrErr(C.ILL_Shape_Mesh(fp(&vertices[0][0]), C.int(len(vertices)),
		(*C.uint32_t)(unsafe.Pointer(&indices[0])), C.int(len(indices)/3)))
}

// Offset returns a new shape that is s moved by offset. s is not consumed.
func (s *Shape) Offset(offset [3]float32) (*Shape, error) {
	return shapeOrErr(C.ILL_Shape_Offset(s.s, fp(&offset[0])))
}

// Release drops this reference to the shape.
func (s *Shape) Release() {
	if s != nil && s.s != nil {
		C.ILL_Shape_Release(s.s)
		s.s = nil
	}
}

// BodySettings describes a body to create. Start from DefaultBodySettings.
type BodySettings struct {
	Shape           *Shape
	Transform       Transform
	LinearVelocity  [3]float32
	AngularVelocity [3]float32
	Motion          int
	Sensor          bool
	AllowSleeping   bool
	Continuous      bool
	AllowedDOFs     uint32
	Friction        float32
	Restitution     float32
	LinearDamping   float32
	AngularDamping  float32
	GravityFactor   float32
	Mass            float32 // <= 0: computed from the shape
}

// DefaultBodySettings is a dynamic body at the origin with Jolt's defaults.
func DefaultBodySettings(shape *Shape) BodySettings {
	var c C.ILL_BodySettings
	C.ILL_BodySettings_Default(&c)
	return BodySettings{
		Shape:          shape,
		Transform:      Transform{Rotation: [4]float32{0, 0, 0, 1}},
		Motion:         int(c.motion),
		AllowSleeping:  c.allowSleeping != 0,
		AllowedDOFs:    uint32(c.allowedDOFs),
		Friction:       float32(c.friction),
		Restitution:    float32(c.restitution),
		LinearDamping:  float32(c.linearDamping),
		AngularDamping: float32(c.angularDamping),
		GravityFactor:  float32(c.gravityFactor),
	}
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// CreateBody adds a body to the world.
func (w *World) CreateBody(s BodySettings) BodyID {
	var c C.ILL_BodySettings
	C.ILL_BodySettings_Default(&c)
	c.shape = s.Shape.s
	t := s.Transform.array()
	for i := range t {
		c.transform[i] = C.float(t[i])
	}
	for i := 0; i < 3; i++ {
		c.linearVelocity[i] = C.float(s.LinearVelocity[i])
		c.angularVelocity[i] = C.float(s.AngularVelocity[i])
	}
	c.motion = C.int(s.Motion)
	c.sensor = cbool(s.Sensor)
	c.allowSleeping = cbool(s.AllowSleeping)
	c.continuous = cbool(s.Continuous)
	c.allowedDOFs = C.uint32_t(s.AllowedDOFs)
	c.friction = C.float(s.Friction)
	c.restitution = C.float(s.Restitution)
	c.linearDamping = C.float(s.LinearDamping)
	c.angularDamping = C.float(s.AngularDamping)
	c.gravityFactor = C.float(s.GravityFactor)
	c.mass = C.float(s.Mass)
	return BodyID(C.ILL_Body_Create(w.w, &c))
}

// DestroyBody removes a body from the world and frees it.
func (w *World) DestroyBody(id BodyID) { C.ILL_Body_Destroy(w.w, C.ILL_BodyID(id)) }

// Transform returns a body's position and rotation.
func (w *World) Transform(id BodyID) Transform {
	var a [7]float32
	C.ILL_Body_GetTransform(w.w, C.ILL_BodyID(id), fp(&a[0]))
	return transformOf(a)
}

// SetTransform teleports a body.
func (w *World) SetTransform(id BodyID, t Transform, activate bool) {
	a := t.array()
	C.ILL_Body_SetTransform(w.w, C.ILL_BodyID(id), fp(&a[0]), cbool(activate))
}

// MoveKinematic moves a kinematic body to t over dt seconds, so it pushes
// dynamic bodies along the way.
func (w *World) MoveKinematic(id BodyID, t Transform, dt float32) {
	a := t.array()
	C.ILL_Body_MoveKinematic(w.w, C.ILL_BodyID(id), fp(&a[0]), C.float(dt))
}

// Velocity returns a body's linear and angular velocity.
func (w *World) Velocity(id BodyID) (linear, angular [3]float32) {
	var a [6]float32
	C.ILL_Body_GetVelocity(w.w, C.ILL_BodyID(id), fp(&a[0]))
	return [3]float32{a[0], a[1], a[2]}, [3]float32{a[3], a[4], a[5]}
}

// SetVelocity sets a body's linear and angular velocity and wakes it.
func (w *World) SetVelocity(id BodyID, linear, angular [3]float32) {
	a := [6]float32{linear[0], linear[1], linear[2], angular[0], angular[1], angular[2]}
	C.ILL_Body_SetVelocity(w.w, C.ILL_BodyID(id), fp(&a[0]))
}

// AddForce applies a force (Newtons) at the center of mass for the next step.
func (w *World) AddForce(id BodyID, f [3]float32) {
	C.ILL_Body_AddForce(w.w, C.ILL_BodyID(id), fp(&f[0]))
}

// AddTorque applies a torque for the next step.
func (w *World) AddTorque(id BodyID, t [3]float32) {
	C.ILL_Body_AddTorque(w.w, C.ILL_BodyID(id), fp(&t[0]))
}

// AddImpulse changes a body's momentum immediately.
func (w *World) AddImpulse(id BodyID, i [3]float32) {
	C.ILL_Body_AddImpulse(w.w, C.ILL_BodyID(id), fp(&i[0]))
}

// AddAngularImpulse changes a body's angular momentum immediately.
func (w *World) AddAngularImpulse(id BodyID, i [3]float32) {
	C.ILL_Body_AddAngularImpulse(w.w, C.ILL_BodyID(id), fp(&i[0]))
}

// IsActive reports whether a body is awake.
func (w *World) IsActive(id BodyID) bool { return C.ILL_Body_IsActive(w.w, C.ILL_BodyID(id)) != 0 }

// Activate wakes a body.
func (w *World) Activate(id BodyID) { C.ILL_Body_Activate(w.w, C.ILL_BodyID(id)) }

// Contact is a contact beginning or ending between two bodies.
type Contact struct {
	Body1, Body2 BodyID
	Began        bool
	Point        [3]float32 // world space; only set when Began
	Normal       [3]float32 // from Body1 toward Body2; only set when Began
}

// DrainContacts appends the contacts since the last call to buf.
func (w *World) DrainContacts(buf []Contact) []Contact {
	for {
		n := int(C.ILL_World_DrainContacts(w.w, &w.contacts[0], C.int(len(w.contacts))))
		for _, c := range w.contacts[:n] {
			buf = append(buf, Contact{
				Body1:  BodyID(c.body1),
				Body2:  BodyID(c.body2),
				Began:  c.kind == C.ILL_CONTACT_BEGIN,
				Point:  [3]float32{float32(c.point[0]), float32(c.point[1]), float32(c.point[2])},
				Normal: [3]float32{float32(c.normal[0]), float32(c.normal[1]), float32(c.normal[2])},
			})
		}
		if n < len(w.contacts) {
			return buf
		}
	}
}

// RayHit is the closest body a ray hit.
type RayHit struct {
	Body     BodyID
	Fraction float32 // along the ray, 0..1
	Point    [3]float32
	Normal   [3]float32
}

// CastRay casts from origin along dir (whose length is the maximum distance),
// skipping the body ignore (InvalidBody skips nothing).
func (w *World) CastRay(origin, dir [3]float32, ignore BodyID) (RayHit, bool) {
	var h C.ILL_RayHit
	if C.ILL_World_CastRay(w.w, fp(&origin[0]), fp(&dir[0]), C.ILL_BodyID(ignore), &h) == 0 {
		return RayHit{}, false
	}
	return RayHit{
		Body:     BodyID(h.body),
		Fraction: float32(h.fraction),
		Point:    [3]float32{float32(h.point[0]), float32(h.point[1]), float32(h.point[2])},
		Normal:   [3]float32{float32(h.normal[0]), float32(h.normal[1]), float32(h.normal[2])},
	}, true
}

// Character is a kinematic character controller: it slides along walls,
// walks up slopes and stairs, and stands on moving platforms.
type Character struct {
	c *C.ILL_Character
}

// NewCharacter creates a character. shape should be a capsule whose bottom
// sphere has radius supportRadius; contacts below its center count as ground.
func (w *World) NewCharacter(shape *Shape, position [3]float32, maxSlope, supportRadius float32) *Character {
	return &Character{c: C.ILL_Character_New(w.w, shape.s, fp(&position[0]), C.float(maxSlope), C.float(supportRadius))}
}

// Close removes the character from the world.
func (c *Character) Close() {
	if c.c != nil {
		C.ILL_Character_Delete(c.c)
		c.c = nil
	}
}

// Update moves the character by its velocity over dt, climbing steps up to
// stepUp high.
func (c *Character) Update(dt, stepUp float32) {
	C.ILL_Character_Update(c.c, C.float(dt), C.float(stepUp))
}

// Position returns the character's position.
func (c *Character) Position() (p [3]float32) {
	C.ILL_Character_GetPosition(c.c, fp(&p[0]))
	return p
}

// SetPosition teleports the character.
func (c *Character) SetPosition(p [3]float32) { C.ILL_Character_SetPosition(c.c, fp(&p[0])) }

// Velocity returns the character's velocity.
func (c *Character) Velocity() (v [3]float32) {
	C.ILL_Character_GetVelocity(c.c, fp(&v[0]))
	return v
}

// SetVelocity sets the velocity the next Update moves by.
func (c *Character) SetVelocity(v [3]float32) { C.ILL_Character_SetVelocity(c.c, fp(&v[0])) }

// GroundVelocity is the velocity of whatever the character stands on.
func (c *Character) GroundVelocity() (v [3]float32) {
	C.ILL_Character_GetGroundVelocity(c.c, fp(&v[0]))
	return v
}

// GroundNormal is the surface normal of whatever the character stands on.
func (c *Character) GroundNormal() (n [3]float32) {
	C.ILL_Character_GetGroundNormal(c.c, fp(&n[0]))
	return n
}

// Supported reports whether the character stands on something walkable.
func (c *Character) Supported() bool { return C.ILL_Character_IsSupported(c.c) != 0 }

// InnerBody is the rigid body that represents the character to other bodies.
func (c *Character) InnerBody() BodyID { return BodyID(C.ILL_Character_InnerBody(c.c)) }
