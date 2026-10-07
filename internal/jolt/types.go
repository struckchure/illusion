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
	Character       bool // character/ragdoll body: yields to vehicles
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

// Vehicle controllers for VehicleSettings.Controller.
const (
	Wheeled    = 0
	Motorcycle = 1
)

// Wheel collision testers for VehicleSettings.Tester.
const (
	TestCylinder = 0
	TestRay      = 1
	TestSphere   = 2
)

// WheelSettings describes one wheel of a vehicle, in the body's local space.
// Start from DefaultWheelSettings. Its layout matches glue.h's ILL_WheelDesc.
type WheelSettings struct {
	Position      [3]float32 // where the suspension is attached
	SuspensionDir [3]float32 // pointing down
	SteeringAxis  [3]float32 // pointing up
	WheelUp       [3]float32 // up at neutral steering
	WheelForward  [3]float32 // forward at neutral steering
	// ModelRight and ModelUp are the axes of the wheel's model that
	// VehicleWheels turns to face the wheel's right and up.
	ModelRight [3]float32
	ModelUp    [3]float32

	Radius, Width                float32
	SuspensionMin, SuspensionMax float32 // suspension length fully raised and fully drooped
	Preload                      float32
	Frequency, Damping           float32 // the suspension spring: Hz, and 0..1 (or see Stiffness)
	MaxSteer                     float32 // radians; negative steers the other way
	MaxBrakeTorque               float32
	MaxHandBrakeTorque           float32
	Inertia, AngularDamping      float32
	LongitudinalGrip             float32 // scales the tire's forward friction curve
	LateralGrip                  float32 // scales the tire's sideways friction curve
	// Stiffness, if 1, makes Frequency the spring's stiffness (N/m) and
	// Damping its damping (N·s/m).
	Stiffness int32
}

// VehicleSettings describes a vehicle's controller, engine and gearbox. Start
// from DefaultVehicleSettings. Its layout matches glue.h's ILL_VehicleDesc.
type VehicleSettings struct {
	Up, Forward      [3]float32
	MaxPitchRoll     float32 // radians; pi = no limit
	Controller       int32   // Wheeled or Motorcycle
	Tester           int32   // TestCylinder, TestRay or TestSphere
	MaxTorque        float32 // Nm
	MinRPM, MaxRPM   float32
	EngineInertia    float32
	EngineDamping    float32
	Gears            [8]float32 // forward gear ratios, the first NumGears used
	NumGears         int32
	ReverseGear      float32
	ShiftUpRPM       float32
	ShiftDownRPM     float32
	ClutchStrength   float32
	SwitchTime       float32
	LimitedSlipRatio float32 // between differentials
	MaxLean          float32 // Motorcycle only, radians
	LeanSpring       float32
	LeanDamping      float32
}

// Differential sends the engine's torque to a pair of wheels. Start from
// DefaultDifferential. Its layout matches glue.h's ILL_Differential.
type Differential struct {
	Left, Right int32   // wheel indices, -1 for none
	Ratio       float32 // gearbox to wheel rotation
	Split       float32 // 0 = all to the left wheel, 1 = all to the right
	TorqueRatio float32 // share of the engine's torque
	LimitedSlip float32
}

// AntiRollBar ties a pair of wheels' suspension together. Its layout matches
// glue.h's ILL_AntiRollBar.
type AntiRollBar struct {
	Left, Right int32
	Stiffness   float32 // N/m
}

// WheelState is a wheel as last simulated.
type WheelState struct {
	Transform  Transform // the wheel model's, in the body's space
	Spin       float32   // angular velocity, rad/s
	Steer      float32   // radians
	Suspension float32   // length, m
	Contact    bool
}

// VehicleStatus is a vehicle's engine and motion as last simulated.
type VehicleStatus struct {
	RPM      float32
	Gear     int     // -1 reverse, 0 neutral, 1 first...
	Speed    float32 // along the vehicle's forward, m/s
	Touching int     // wheels touching something
}

const wheelStateFloats = 11 // ILL_WHEEL_STATE

func wheelStates(buf []WheelState, a []float32) []WheelState {
	for i := 0; i+wheelStateFloats <= len(a); i += wheelStateFloats {
		var t [7]float32
		copy(t[:], a[i:i+7])
		buf = append(buf, WheelState{
			Transform:  transformOf(t),
			Spin:       a[i+7],
			Steer:      a[i+8],
			Suspension: a[i+9],
			Contact:    a[i+10] != 0,
		})
	}
	return buf
}

func vehicleStatus(a [4]float32) VehicleStatus {
	return VehicleStatus{RPM: a[0], Gear: int(a[1]), Speed: a[2], Touching: int(a[3])}
}
