//go:build !js

// The cgo binding, compiling Jolt from source with the Go toolchain.

package jolt

/*
#cgo CXXFLAGS: -std=c++17 -O2 -DNDEBUG -I${SRCDIR}/third_party/JoltPhysics -Wno-unused-parameter
#cgo darwin LDFLAGS: -lc++
#cgo linux LDFLAGS: -lstdc++ -lm -lpthread
#cgo windows LDFLAGS: -static-libgcc -static-libstdc++ -Wl,-Bstatic -lstdc++ -lwinpthread -Wl,-Bdynamic
#include <stdlib.h>
#include "glue.h"

// Called for every character every step: these only read or write the
// vectors they're given, so the vectors needn't go to the heap, and they
// don't call back into Go.
#cgo noescape ILL_Character_GetPosition
#cgo nocallback ILL_Character_GetPosition
#cgo noescape ILL_Character_SetPosition
#cgo nocallback ILL_Character_SetPosition
#cgo noescape ILL_Character_GetVelocity
#cgo nocallback ILL_Character_GetVelocity
#cgo noescape ILL_Character_SetVelocity
#cgo nocallback ILL_Character_SetVelocity
#cgo noescape ILL_Character_GetGroundVelocity
#cgo nocallback ILL_Character_GetGroundVelocity
#cgo noescape ILL_Character_GetGroundNormal
#cgo nocallback ILL_Character_GetGroundNormal
#cgo noescape ILL_World_CastRay
#cgo nocallback ILL_World_CastRay
#cgo noescape ILL_World_OverlapCapsule
#cgo nocallback ILL_World_OverlapCapsule
#cgo noescape ILL_World_SweepCapsule
#cgo nocallback ILL_World_SweepCapsule
*/
import "C"

import "unsafe"

// The constants in types.go must match glue.h.
const _ = uint(Static-C.ILL_STATIC) + uint(C.ILL_STATIC-Static) +
	uint(Kinematic-C.ILL_KINEMATIC) + uint(C.ILL_KINEMATIC-Kinematic) +
	uint(Dynamic-C.ILL_DYNAMIC) + uint(C.ILL_DYNAMIC-Dynamic) +
	uint(InvalidBody-C.ILL_INVALID_BODY) + uint(C.ILL_INVALID_BODY-InvalidBody)

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
		return errOverflow
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

// OverlapCapsule reports significant penetration, ignoring touching support surfaces.
func (w *World) OverlapCapsule(center [3]float32, radius, height float32, ignore BodyID) bool {
	return C.ILL_World_OverlapCapsule(w.w, fp(&center[0]), C.float(radius), C.float(height), C.ILL_BodyID(ignore)) != 0
}
func (w *World) SweepCapsule(center, delta [3]float32, radius, height float32, ignore BodyID) (RayHit, bool) {
	var h C.ILL_RayHit
	if C.ILL_World_SweepCapsule(w.w, fp(&center[0]), fp(&delta[0]), C.float(radius), C.float(height), C.ILL_BodyID(ignore), &h) == 0 {
		return RayHit{}, false
	}
	return RayHit{Body: BodyID(h.body), Fraction: float32(h.fraction), Point: [3]float32{float32(h.point[0]), float32(h.point[1]), float32(h.point[2])}, Normal: [3]float32{float32(h.normal[0]), float32(h.normal[1]), float32(h.normal[2])}}, true
}
func (c *Character) UpdateControlled(dt float32) { C.ILL_Character_UpdateControlled(c.c, C.float(dt)) }

// The vehicle structs in types.go must match glue.h's layouts.
const _ = uint(unsafe.Sizeof(WheelSettings{})-unsafe.Sizeof(C.ILL_WheelDesc{})) + uint(unsafe.Sizeof(C.ILL_WheelDesc{})-unsafe.Sizeof(WheelSettings{})) +
	uint(unsafe.Sizeof(VehicleSettings{})-unsafe.Sizeof(C.ILL_VehicleDesc{})) + uint(unsafe.Sizeof(C.ILL_VehicleDesc{})-unsafe.Sizeof(VehicleSettings{})) +
	uint(unsafe.Sizeof(Differential{})-unsafe.Sizeof(C.ILL_Differential{})) + uint(unsafe.Sizeof(C.ILL_Differential{})-unsafe.Sizeof(Differential{})) +
	uint(unsafe.Sizeof(AntiRollBar{})-unsafe.Sizeof(C.ILL_AntiRollBar{})) + uint(unsafe.Sizeof(C.ILL_AntiRollBar{})-unsafe.Sizeof(AntiRollBar{})) +
	uint(wheelStateFloats-C.ILL_WHEEL_STATE) + uint(C.ILL_WHEEL_STATE-wheelStateFloats) +
	uint(Wheeled-C.ILL_WHEELED) + uint(C.ILL_WHEELED-Wheeled) + uint(Motorcycle-C.ILL_MOTORCYCLE) + uint(C.ILL_MOTORCYCLE-Motorcycle) +
	uint(TestCylinder-C.ILL_TEST_CYLINDER) + uint(C.ILL_TEST_CYLINDER-TestCylinder) +
	uint(TestRay-C.ILL_TEST_RAY) + uint(C.ILL_TEST_RAY-TestRay) + uint(TestSphere-C.ILL_TEST_SPHERE) + uint(C.ILL_TEST_SPHERE-TestSphere)

// OffsetCenterOfMass returns a new shape that is s with its center of mass
// moved by offset. s is not consumed.
func (s *Shape) OffsetCenterOfMass(offset [3]float32) (*Shape, error) {
	return shapeOrErr(C.ILL_Shape_OffsetCenterOfMass(s.s, fp(&offset[0])))
}

// CenterOfMass is where the shape's center of mass is, in its own space.
func (s *Shape) CenterOfMass() (c [3]float32) {
	C.ILL_Shape_GetCenterOfMass(s.s, fp(&c[0]))
	return c
}

// DefaultWheelSettings is Jolt's default wheel.
func DefaultWheelSettings() (s WheelSettings) {
	C.ILL_WheelDesc_Default((*C.ILL_WheelDesc)(unsafe.Pointer(&s)))
	return s
}

// DefaultVehicleSettings is Jolt's default car: +Y up, +Z forward.
func DefaultVehicleSettings() (s VehicleSettings) {
	C.ILL_VehicleDesc_Default((*C.ILL_VehicleDesc)(unsafe.Pointer(&s)))
	return s
}

// DefaultDifferential is Jolt's default differential, with no wheels.
func DefaultDifferential() (d Differential) {
	C.ILL_Differential_Default((*C.ILL_Differential)(unsafe.Pointer(&d)))
	return d
}

// CreateVehicle makes a dynamic body a vehicle. It reports false if the body
// isn't dynamic, already is one, or wheels is empty.
func (w *World) CreateVehicle(body BodyID, s VehicleSettings, wheels []WheelSettings, diffs []Differential, bars []AntiRollBar) bool {
	if len(wheels) == 0 {
		return false
	}
	var d *C.ILL_Differential
	if len(diffs) > 0 {
		d = (*C.ILL_Differential)(unsafe.Pointer(&diffs[0]))
	}
	var b *C.ILL_AntiRollBar
	if len(bars) > 0 {
		b = (*C.ILL_AntiRollBar)(unsafe.Pointer(&bars[0]))
	}
	return C.ILL_Vehicle_Create(w.w, C.ILL_BodyID(body), (*C.ILL_VehicleDesc)(unsafe.Pointer(&s)),
		(*C.ILL_WheelDesc)(unsafe.Pointer(&wheels[0])), C.int(len(wheels)), d, C.int(len(diffs)), b, C.int(len(bars))) != 0
}

// DestroyVehicle takes a body's vehicle away, leaving the body. Destroying the
// body does this too.
func (w *World) DestroyVehicle(body BodyID) { C.ILL_Vehicle_Destroy(w.w, C.ILL_BodyID(body)) }

// SetVehicleInput sets what the driver does until it's next set: forward and
// right from -1 to 1, brake and handBrake from 0 to 1.
func (w *World) SetVehicleInput(body BodyID, forward, right, brake, handBrake float32) {
	C.ILL_Vehicle_SetInput(w.w, C.ILL_BodyID(body), C.float(forward), C.float(right), C.float(brake), C.float(handBrake))
}

// VehicleWheels appends a vehicle's wheels to buf.
func (w *World) VehicleWheels(body BodyID, buf []WheelState) []WheelState {
	var a [8 * wheelStateFloats]float32
	n := int(C.ILL_Vehicle_GetWheels(w.w, C.ILL_BodyID(body), fp(&a[0]), 8))
	if n > 8 {
		big := make([]float32, n*wheelStateFloats)
		C.ILL_Vehicle_GetWheels(w.w, C.ILL_BodyID(body), fp(&big[0]), C.int(n))
		return wheelStates(buf, big)
	}
	return wheelStates(buf, a[:n*wheelStateFloats])
}

// VehicleStatus returns a vehicle's engine and motion.
func (w *World) VehicleStatus(body BodyID) VehicleStatus {
	var a [4]float32
	C.ILL_Vehicle_GetStatus(w.w, C.ILL_BodyID(body), fp(&a[0]))
	return vehicleStatus(a)
}
