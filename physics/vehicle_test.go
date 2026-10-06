package physics

import (
	"math"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/transform"
)

// car is a 1200 kg four-wheel-drive car facing +Z, its wheels resting on the
// ground when its origin is 0.8 m up.
func car(rb RigidBody, tune ...func(*Vehicle, *Mass)) []illusion.Component {
	var wheels []Wheel
	for _, z := range []float32{1.3, -1.3} {
		for _, x := range []float32{0.9, -0.9} {
			steer := float32(0)
			if z > 0 {
				steer = 0.5
			}
			wheels = append(wheels, Wheel{Position: rl.Vector3{X: x, Y: -0.1, Z: z}, Radius: 0.35, Width: 0.2,
				SuspensionMin: 0.2, SuspensionMax: 0.45, MaxSteer: steer, HandBrake: 3000})
		}
	}
	v := Vehicle{
		Wheels:        wheels,
		Differentials: []Differential{{Left: 0, Right: 1}, {Left: 2, Right: 3}},
		AntiRollBars:  []AntiRollBar{{Left: 0, Right: 1}, {Left: 2, Right: 3}},
	}
	mass := Mass(1200)
	for _, f := range tune {
		f(&v, &mass)
	}
	return []illusion.Component{
		illusion.C(tag("car")),
		illusion.C(rb),
		illusion.C(Cuboid(1.8, 0.6, 4).WithCenterOfMass(rl.Vector3{Y: -0.3})),
		illusion.C(mass),
		illusion.C(transform.FromXYZ(0, 0.8, 0)),
		illusion.C(v),
		illusion.C(VehicleInput{}),
		illusion.C(VehicleState{}),
		illusion.C(Interpolated{}),
	}
}

func TestVehicleDrivesOnItsWheels(t *testing.T) {
	app := newApp(t, func(cmd *illusion.Commands) { cmd.Spawn(car(Dynamic)...) })
	run(app, 1)
	e := find(app, "car")
	st := ecs.NewMap[VehicleState](app.World).Get(e)
	if len(st.Wheels) != 4 || st.Touching != 4 {
		t.Fatalf("parked car should have 4 wheels down, got %d of %d", st.Touching, len(st.Wheels))
	}
	for i, w := range st.Wheels {
		// The wheel's center is a radius off the floor.
		y := translation(app, e).Y + w.Transform.Translation.Y
		if y < 0.3 || y > 0.4 {
			t.Errorf("wheel %d center at y=%v, want about 0.35", i, y)
		}
	}

	ecs.NewMap[VehicleInput](app.World).Get(e).Forward = 1
	run(app, 2)
	if z := translation(app, e).Z; z < 4 {
		t.Fatalf("car should drive forward along +Z, got z=%v", z)
	}
	if st.Speed < 2 || st.Gear < 1 || st.Wheels[0].Spin <= 0 {
		t.Fatalf("car should be moving in gear with its wheels turning, got %+v", st)
	}
}

func TestVehicleRunsOnlyWhileDynamic(t *testing.T) {
	app := newApp(t, func(cmd *illusion.Commands) { cmd.Spawn(car(Static)...) })
	run(app, 0.5)
	e := find(app, "car")
	st := ecs.NewMap[VehicleState](app.World).Get(e)
	in := ecs.NewMap[VehicleInput](app.World).Get(e)
	in.Forward = 1
	run(app, 0.5)
	if len(st.Wheels) != 0 || translation(app, e).Z != 0 {
		t.Fatalf("a Static vehicle shouldn't simulate, got %+v at %v", st, translation(app, e))
	}

	*ecs.NewMap[RigidBody](app.World).Get(e) = Dynamic
	run(app, 1.5)
	if len(st.Wheels) != 4 || translation(app, e).Z < 1 {
		t.Fatalf("switched Dynamic, it should drive: %+v at %v", st, translation(app, e))
	}

	in.Forward, in.HandBrake = 0, 1
	*ecs.NewMap[RigidBody](app.World).Get(e) = Static
	run(app, 0.1)
	at := translation(app, e)
	run(app, 0.5)
	if translation(app, e) != at {
		t.Fatal("switched back to Static, it should stay put")
	}
}

func TestDespawningAVehicle(t *testing.T) {
	app := newApp(t, func(cmd *illusion.Commands) { cmd.Spawn(car(Dynamic)...) })
	run(app, 0.5)
	e := find(app, "car")
	ecs.NewMap[VehicleInput](app.World).Get(e).Forward = 1
	app.AddSystems(illusion.Update, illusion.Fn1(func(cmd *illusion.Commands) { cmd.Despawn(e) }).RunIf(illusion.Once()))
	run(app, 0.5) // stepping a freed vehicle would crash
}

func TestInterpolatedBlendsSteps(t *testing.T) {
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(tag("ball")), illusion.C(Dynamic), illusion.C(Sphere(0.5)),
			illusion.C(transform.FromXYZ(0, 10, 0)), illusion.C(Interpolated{}))
	})
	run(app, 0.5)
	e := find(app, "ball")
	in := ecs.NewMap[Interpolated](app.World).Get(e)
	if in.Previous.Translation.Y <= in.Current.Translation.Y {
		t.Fatalf("a falling ball's previous pose should be above its current: %v, %v",
			in.Previous.Translation, in.Current.Translation)
	}
	if in.Current.Translation != translation(app, e) {
		t.Fatal("Current should be the simulated pose")
	}
	mid := in.At(0.5).Translation.Y
	if mid >= in.Previous.Translation.Y || mid <= in.Current.Translation.Y {
		t.Fatalf("halfway should be between the two poses, got %v", mid)
	}

	ecs.NewMap[transform.Transform](app.World).Get(e).Translation = rl.Vector3{X: 20, Y: 10}
	run(app, 1.0/60)
	if in.Previous.Translation.X != 20 {
		t.Fatalf("a teleport shouldn't slide from the old pose, previous is %v", in.Previous.Translation)
	}
}

func TestStiffSpringsSinkByTheirLoad(t *testing.T) {
	const mass, k = 1000, 50000 // 2500 N on each of 4 springs: 5 cm each
	comps := car(Dynamic, func(v *Vehicle, m *Mass) {
		*m = mass
		for j := range v.Wheels {
			v.Wheels[j].Stiffness, v.Wheels[j].DampingRate = k, 6000
		}
	})
	app := newApp(t, func(cmd *illusion.Commands) { cmd.Spawn(comps...) })
	run(app, 3)
	st := ecs.NewMap[VehicleState](app.World).Get(find(app, "car"))
	want := float32(0.45 - mass*9.81/4/k)
	for i, w := range st.Wheels {
		if d := w.Suspension - want; d > 0.01 || d < -0.01 {
			t.Errorf("wheel %d suspension %v, want %v: drooped fully less its load over its stiffness", i, w.Suspension, want)
		}
	}
}

func TestCenterOfMassIsWhereItsPut(t *testing.T) {
	// A post standing on its end, tipped 20°: weighted at its middle it
	// would fall over, but its weight is put at its foot (not moved down from
	// its middle by that much), so it rocks back up.
	tip := float32(20 * math.Pi / 180)
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(tag("post")), illusion.C(Dynamic),
			illusion.C(Cuboid(0.4, 2, 0.4).WithOffset(rl.Vector3{Y: 1}).WithCenterOfMass(rl.Vector3{Y: 0.05})),
			illusion.C(transform.FromXYZ(0, 0.15, 0).WithRotation(rl.QuaternionFromAxisAngle(rl.Vector3{Z: 1}, tip))))
	})
	run(app, 4)
	r := ecs.NewMap[transform.Transform](app.World).Get(find(app, "post")).Rotation
	if up := rl.Vector3RotateByQuaternion(rl.Vector3{Y: 1}, r); up.Y < 0.99 {
		t.Fatalf("weighted at its foot, it should stand back up, up is %v", up)
	}
}
