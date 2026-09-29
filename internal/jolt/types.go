// Package jolt is a thin binding over Jolt Physics. Jolt is vendored in
// third_party (see update.sh); glue.cpp exposes the small C API declared in
// glue.h. Native builds compile it with cgo (jolt.go). Browser builds call a
// separate wasm module built with emscripten (jolt_js.go, see web/README.md).
//
// Vectors are [3]float32 and transforms are position plus quaternion (x, y, z,
// w), so callers can convert from their own math types without copying
// through C structs.
package jolt

import "errors"

// Declarations shared by the cgo binding (jolt.go) and the browser one
// (jolt_js.go). Constants match glue.h; jolt.go checks that at compile time.

// Motion types.
const (
	Static    = 0
	Kinematic = 1
	Dynamic   = 2
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
const InvalidBody BodyID = 0xffffffff

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

var (
	errShape    = errors.New("jolt: invalid shape")
	errOverflow = errors.New("jolt: physics update overflowed its buffers; raise the world's body limit")
)

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

// Contact is a contact beginning or ending between two bodies.
type Contact struct {
	Body1, Body2 BodyID
	Began        bool
	Point        [3]float32 // world space; only set when Began
	Normal       [3]float32 // from Body1 toward Body2; only set when Began
}

// RayHit is the closest body a ray hit.
type RayHit struct {
	Body     BodyID
	Fraction float32 // along the ray, 0..1
	Point    [3]float32
	Normal   [3]float32
}
