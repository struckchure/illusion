package physics

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/internal/jolt"
	"github.com/struckchure/illusion/transform"
)

// Vehicle puts a Dynamic body on wheels (Jolt's VehicleConstraint): each
// wheel casts against the world, rides on a sprung suspension and grips with
// a tire, and an engine drives them through a gearbox and differentials. Set
// [VehicleInput] to drive it, and read [VehicleState] for its wheels.
//
// It's ignored while the body is Static or Kinematic, so a parked vehicle can
// be made Static and woken by switching back to Dynamic. Vectors are in the
// body's local space, which faces Forward. Replacing the Vehicle (with new
// slices) rebuilds the body; changing its slices in place does nothing.
type Vehicle struct {
	Wheels        []Wheel
	Engine        Engine
	Transmission  Transmission
	Differentials []Differential
	AntiRollBars  []AntiRollBar
	// Up and Forward are the body's up and forward; zero means +Y and +Z.
	Up, Forward rl.Vector3
	// MaxPitchRoll keeps the vehicle from tipping further than this (radians);
	// 0 means no limit.
	MaxPitchRoll float32
	// Lean makes a two-wheeled vehicle a motorcycle, which leans into turns
	// and stays up by itself.
	Lean *Lean
	// Tester is how wheels find the ground.
	Tester WheelTester
}

// Wheel is one of a [Vehicle]'s wheels. Zero values mean Jolt's defaults
// where zero wouldn't work.
type Wheel struct {
	// Position is where the suspension is attached: the wheel's center with
	// the suspension fully raised is SuspensionMin below it.
	Position      rl.Vector3
	Radius, Width float32 // 0 means 0.3 and 0.1
	// SuspensionMin and SuspensionMax are how far the wheel's center hangs
	// below Position fully raised and fully drooped. A zero SuspensionMax
	// means SuspensionMin + 0.2.
	SuspensionMin, SuspensionMax float32
	// Frequency (Hz) and Damping (0..1) make the suspension's spring; 0 means
	// 1.5 Hz and 0.5. Jolt sets the spring's stiffness from the mass at each
	// wheel's contact, so how far the vehicle sinks on it depends on its
	// shape and its number of wheels.
	Frequency, Damping float32
	// Stiffness (N/m) and DampingRate (N·s/m), when Stiffness is set, make
	// the spring instead, so it sinks by exactly its load over Stiffness.
	Stiffness, DampingRate float32
	// MaxSteer is how far the wheel steers (radians, up to π/2); negative
	// steers it the other way, as a rear axle does to turn tighter.
	MaxSteer float32
	// Brake is the most torque the brakes put on it (Nm); 0 means 1500.
	Brake float32
	// HandBrake is the most torque the hand brake puts on it (Nm); 0 means
	// it has none.
	HandBrake float32
	// Inertia is the wheel's moment of inertia (kg m²); 0 means 0.9.
	Inertia float32
	// Grip scales the tire's friction, forward and sideways; 0 means 1.
	Grip float32
	// SuspensionDir points down the suspension, and SteeringAxis up the axis
	// the wheel steers around; zero means straight down and straight up. A
	// bike's fork tilts both.
	SuspensionDir, SteeringAxis rl.Vector3
	// ModelRight and ModelUp are the axes of the wheel's model that
	// [VehicleState] turns to face the wheel's right and up; zero means the
	// vehicle's. One model can serve both sides of a vehicle by flipping
	// ModelRight for the wheels on one side.
	ModelRight, ModelUp rl.Vector3
}

// Engine is a [Vehicle]'s engine. Zero values mean Jolt's defaults: 500 Nm
// from 1000 to 6000 rpm.
type Engine struct {
	MaxTorque      float32
	MinRPM, MaxRPM float32
}

// Transmission is a [Vehicle]'s automatic gearbox. Zero values mean Jolt's
// defaults: five gears, shifting up at 4000 rpm and down at 2000.
type Transmission struct {
	Gears                    []float32 // forward gear ratios, first gear first
	Reverse                  float32   // reverse gear ratio
	ShiftUpRPM, ShiftDownRPM float32
	ClutchStrength           float32
}

// Differential drives a pair of a [Vehicle]'s wheels, by index into Wheels;
// -1 means no wheel. A vehicle with none doesn't drive.
type Differential struct {
	Left, Right int
	// Ratio is how many times the wheels turn slower than the gearbox; 0
	// means 3.42.
	Ratio float32
	// Split is the share of torque that goes to the right wheel; 0 means an
	// even 0.5.
	Split float32
	// TorqueRatio is this differential's share of the engine's torque; 0
	// means an even share among all the differentials.
	TorqueRatio float32
	// LimitedSlip is the most one wheel turns faster than the other before
	// torque goes to the slower one; 0 means 1.4.
	LimitedSlip float32
}

// AntiRollBar ties the suspension of two of a [Vehicle]'s wheels together,
// keeping it flatter in turns.
type AntiRollBar struct {
	Left, Right int
	Stiffness   float32 // N/m; 0 means 1000
}

// Lean is a motorcycle's balance. Zero values mean Jolt's defaults.
type Lean struct {
	MaxAngle        float32 // radians; 0 means 45°
	Spring, Damping float32
}

// WheelTester is how a [Vehicle]'s wheels find the ground.
type WheelTester uint8

const (
	// CastCylinder sweeps each wheel's cylinder: the most accurate.
	CastCylinder WheelTester = iota
	// CastRay casts a ray down each suspension: the cheapest, but wheels drop
	// into gaps narrower than they are.
	CastRay
	// CastSphere sweeps a sphere as wide as the narrowest wheel.
	CastSphere
)

// VehicleInput is what drives a [Vehicle], set each frame. Forward and Right
// run from -1 to 1 (pushing Forward against the direction of travel brakes,
// then reverses), Brake and HandBrake from 0 to 1.
type VehicleInput struct {
	Forward, Right   float32
	Brake, HandBrake float32
}

// VehicleState is a [Vehicle] as last simulated, written after every step
// while its body is Dynamic.
type VehicleState struct {
	Wheels   []WheelState
	RPM      float32
	Gear     int     // -1 reverse, 0 neutral, 1 first...
	Speed    float32 // along the vehicle's forward, m/s
	Touching int     // wheels on something
}

// WheelState is a wheel as last simulated.
type WheelState struct {
	// Transform places the wheel's model relative to the body: its suspension,
	// steering and spin.
	Transform  transform.Transform
	Spin       float32 // angular velocity, rad/s, positive rolling forward
	Steer      float32 // radians
	Suspension float32 // length, m
	Contact    bool
}

// vehicleKey identifies a Vehicle. Slices are compared by identity, like
// colliderKey's.
type vehicleKey struct {
	wheels             *Wheel
	nWheels            int
	diffs              *Differential
	nDiffs             int
	bars               *AntiRollBar
	nBars              int
	gears              *float32
	nGears             int
	engine             Engine
	reverse            float32
	shiftUp, shiftDown float32
	clutch             float32
	up, forward        rl.Vector3
	maxPitchRoll       float32
	lean               *Lean
	tester             WheelTester
}

func vehicleKeyOf(v *Vehicle) vehicleKey {
	k := vehicleKey{nWheels: len(v.Wheels), nDiffs: len(v.Differentials), nBars: len(v.AntiRollBars),
		nGears: len(v.Transmission.Gears), engine: v.Engine, reverse: v.Transmission.Reverse,
		shiftUp: v.Transmission.ShiftUpRPM, shiftDown: v.Transmission.ShiftDownRPM,
		clutch: v.Transmission.ClutchStrength, up: v.Up, forward: v.Forward, maxPitchRoll: v.MaxPitchRoll,
		lean: v.Lean, tester: v.Tester}
	if len(v.Wheels) > 0 {
		k.wheels = &v.Wheels[0]
	}
	if len(v.Differentials) > 0 {
		k.diffs = &v.Differentials[0]
	}
	if len(v.AntiRollBars) > 0 {
		k.bars = &v.AntiRollBars[0]
	}
	if len(v.Transmission.Gears) > 0 {
		k.gears = &v.Transmission.Gears[0]
	}
	return k
}

func orDefault(v, def float32) float32 {
	if v == 0 {
		return def
	}
	return v
}

func dirOr(v, def rl.Vector3) [3]float32 {
	if v == (rl.Vector3{}) {
		return v3(def)
	}
	return v3(rl.Vector3Normalize(v))
}

// createVehicle builds v on body id, reporting whether it could.
func createVehicle(w *jolt.World, id jolt.BodyID, v *Vehicle) bool {
	up := rl.Vector3{Y: 1}
	if v.Up != (rl.Vector3{}) {
		up = rl.Vector3Normalize(v.Up)
	}
	forward := rl.Vector3{Z: 1}
	if v.Forward != (rl.Vector3{}) {
		forward = rl.Vector3Normalize(v.Forward)
	}
	right := rl.Vector3CrossProduct(forward, up)

	s := jolt.DefaultVehicleSettings()
	s.Up, s.Forward = v3(up), v3(forward)
	s.MaxPitchRoll = math.Pi
	if v.MaxPitchRoll > 0 {
		s.MaxPitchRoll = v.MaxPitchRoll
	}
	switch v.Tester {
	case CastRay:
		s.Tester = jolt.TestRay
	case CastSphere:
		s.Tester = jolt.TestSphere
	default:
		s.Tester = jolt.TestCylinder
	}
	s.MaxTorque = orDefault(v.Engine.MaxTorque, s.MaxTorque)
	s.MinRPM = orDefault(v.Engine.MinRPM, s.MinRPM)
	s.MaxRPM = max(orDefault(v.Engine.MaxRPM, s.MaxRPM), s.MinRPM+1)
	t := v.Transmission
	if len(t.Gears) > 0 {
		s.NumGears = int32(min(len(t.Gears), len(s.Gears)))
		copy(s.Gears[:], t.Gears)
	}
	s.ReverseGear = -float32(math.Abs(float64(orDefault(t.Reverse, s.ReverseGear))))
	s.ShiftUpRPM = orDefault(t.ShiftUpRPM, s.ShiftUpRPM)
	s.ShiftDownRPM = orDefault(t.ShiftDownRPM, s.ShiftDownRPM)
	// Jolt's gearbox needs MinRPM <= down < up < MaxRPM.
	s.ShiftUpRPM = min(s.ShiftUpRPM, s.MaxRPM-1)
	s.ShiftDownRPM = min(s.ShiftDownRPM, s.ShiftUpRPM-1)
	s.ClutchStrength = orDefault(t.ClutchStrength, s.ClutchStrength)
	if v.Lean != nil {
		s.Controller = jolt.Motorcycle
		s.MaxLean = orDefault(v.Lean.MaxAngle, s.MaxLean)
		s.LeanSpring = orDefault(v.Lean.Spring, s.LeanSpring)
		s.LeanDamping = orDefault(v.Lean.Damping, s.LeanDamping)
	}

	wheels := make([]jolt.WheelSettings, len(v.Wheels))
	for i, wh := range v.Wheels {
		ws := jolt.DefaultWheelSettings()
		ws.Position = v3(wh.Position)
		ws.SuspensionDir = dirOr(wh.SuspensionDir, rl.Vector3Negate(up))
		ws.SteeringAxis = dirOr(wh.SteeringAxis, up)
		ws.WheelUp, ws.WheelForward = v3(up), v3(forward)
		ws.ModelRight = dirOr(wh.ModelRight, right)
		ws.ModelUp = dirOr(wh.ModelUp, up)
		ws.Radius = orDefault(wh.Radius, ws.Radius)
		ws.Width = orDefault(wh.Width, ws.Width)
		ws.SuspensionMin = max(wh.SuspensionMin, 0)
		ws.SuspensionMax = wh.SuspensionMax
		if ws.SuspensionMax <= ws.SuspensionMin {
			ws.SuspensionMax = ws.SuspensionMin + 0.2
		}
		ws.Preload = 0
		ws.Frequency = orDefault(wh.Frequency, ws.Frequency)
		ws.Damping = orDefault(wh.Damping, ws.Damping)
		if wh.Stiffness > 0 {
			ws.Stiffness, ws.Frequency, ws.Damping = 1, wh.Stiffness, max(wh.DampingRate, 0)
		}
		ws.MaxSteer = max(min(wh.MaxSteer, math.Pi/2), -math.Pi/2)
		ws.MaxBrakeTorque = orDefault(wh.Brake, ws.MaxBrakeTorque)
		ws.MaxHandBrakeTorque = max(wh.HandBrake, 0)
		ws.Inertia = orDefault(wh.Inertia, ws.Inertia)
		ws.LongitudinalGrip = orDefault(wh.Grip, 1)
		ws.LateralGrip = ws.LongitudinalGrip
		wheels[i] = ws
	}

	diffs := make([]jolt.Differential, len(v.Differentials))
	for i, d := range v.Differentials {
		jd := jolt.DefaultDifferential()
		jd.Left, jd.Right = int32(d.Left), int32(d.Right)
		jd.Ratio = orDefault(d.Ratio, jd.Ratio)
		jd.Split = orDefault(d.Split, 0.5)
		jd.TorqueRatio = orDefault(d.TorqueRatio, 1/float32(len(v.Differentials)))
		jd.LimitedSlip = orDefault(d.LimitedSlip, jd.LimitedSlip)
		diffs[i] = jd
	}
	bars := make([]jolt.AntiRollBar, len(v.AntiRollBars))
	for i, b := range v.AntiRollBars {
		bars[i] = jolt.AntiRollBar{Left: int32(b.Left), Right: int32(b.Right), Stiffness: orDefault(b.Stiffness, 1000)}
	}
	return w.CreateVehicle(id, s, wheels, diffs, bars)
}

// pushVehicles hands each vehicle its driver's input.
func pushVehicles(q *illusion.Query2[body, VehicleInput], res *illusion.Res[world]) {
	w := res.Get()
	query := q.Iter()
	for query.Next() {
		b, in := query.Get()
		if b.vehicle {
			w.jolt.SetVehicleInput(b.id, in.Forward, in.Right, in.Brake, in.HandBrake)
		}
	}
}

// pullVehicles writes each simulated vehicle's VehicleState.
func pullVehicles(q *illusion.Query2[body, VehicleState], res *illusion.Res[world], settings *illusion.Res[Settings]) {
	if settings.Get().Paused {
		return
	}
	w := res.Get()
	query := q.Iter()
	for query.Next() {
		b, st := query.Get()
		if !b.vehicle {
			continue
		}
		w.wheels = w.jolt.VehicleWheels(b.id, w.wheels[:0])
		st.Wheels = st.Wheels[:0]
		for _, wh := range w.wheels {
			r := wh.Transform.Rotation
			st.Wheels = append(st.Wheels, WheelState{
				Transform: transform.Transform{
					Translation: vec(wh.Transform.Position),
					Rotation:    rl.Quaternion{X: r[0], Y: r[1], Z: r[2], W: r[3]},
					Scale:       rl.Vector3One(),
				},
				Spin:       wh.Spin,
				Steer:      wh.Steer,
				Suspension: wh.Suspension,
				Contact:    wh.Contact,
			})
		}
		s := w.jolt.VehicleStatus(b.id)
		st.RPM, st.Gear, st.Speed, st.Touching = s.RPM, s.Gear, s.Speed, s.Touching
	}
}

// Interpolated keeps the last two simulated poses of a body, so it can be
// drawn smoothly between fixed steps: show At(FixedTime.Overstep()) rather
// than the Transform, which moves in fixed-step jumps. Moving the Transform
// from game code teleports it, without sliding.
type Interpolated struct {
	Previous, Current transform.Transform
	ready, snap       bool
}

// At is the pose alpha (0..1) of the way from Previous to Current.
func (i Interpolated) At(alpha float32) transform.Transform {
	alpha = max(0, min(1, alpha))
	return transform.Transform{
		Translation: rl.Vector3Lerp(i.Previous.Translation, i.Current.Translation, alpha),
		Rotation:    rl.QuaternionSlerp(quatOr(i.Previous.Rotation), quatOr(i.Current.Rotation), alpha),
		Scale:       rl.Vector3Lerp(i.Previous.Scale, i.Current.Scale, alpha),
	}
}

func quatOr(q rl.Quaternion) rl.Quaternion {
	if q == (rl.Quaternion{}) {
		return rl.QuaternionIdentity()
	}
	return q
}

// pullInterpolated shifts every Interpolated body's poses along by a step.
func pullInterpolated(q *illusion.Query2[Interpolated, transform.Transform], settings *illusion.Res[Settings]) {
	if settings.Get().Paused {
		return
	}
	q.Each(func(_ ecs.Entity, in *Interpolated, tr *transform.Transform) {
		if !in.ready || in.snap {
			in.Previous = *tr
		} else {
			in.Previous = in.Current
		}
		in.Current = *tr
		in.ready, in.snap = true, false
	})
}
