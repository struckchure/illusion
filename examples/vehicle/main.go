// A drivable car on a test track: ramps, bumps and a slalom of cones.
//
// The car is a physics.Vehicle: a dynamic box riding on four sprung wheels,
// with an engine, an automatic gearbox and four-wheel drive. Its root
// entity is the physics body; what you see is a child posed from
// physics.Interpolated, so it moves smoothly between fixed steps, and the
// wheels are its children, posed from physics.VehicleState.
//
// W/S or Up/Down throttle and brake (holding S when stopped reverses), A/D or
// Left/Right steer, Space hand brake, R reset, Escape quit.
package main

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/defaults"
	"github.com/struckchure/illusion/input"
	"github.com/struckchure/illusion/physics"
	"github.com/struckchure/illusion/render"
	"github.com/struckchure/illusion/transform"
	"github.com/struckchure/illusion/window"
)

const (
	wheelRadius = 0.38
	wheelWidth  = 0.28
)

var start = transform.FromXYZ(0, 1.2, 0)

// Car is the vehicle's physics root.
type Car struct{}

// Shell is the car's visible body, a child of Car.
type Shell struct{}

// WheelOf is one of the car's wheel models, a child of Shell.
type WheelOf struct{ Index int }

// Chase is the camera.
type Chase struct{}

func main() {
	illusion.New().
		AddPlugins(
			defaults.Plugins(defaults.Config{
				Window:    window.Config{Title: "Illusion: vehicle", MSAA: true},
				AssetRoot: "examples/assets",
			}),
			physics.Plugin{},
		).
		InsertResource(
			illusion.R(&render.ClearColor{Color: rl.NewColor(160, 196, 222, 255)}),
			illusion.R(&render.AmbientLight{Color: rl.NewColor(210, 220, 240, 255), Brightness: 0.4}),
		).
		AddSystems(illusion.Startup, illusion.Fn3(setup)).
		AddSystems(illusion.Update,
			illusion.Chain(illusion.Fn3(drive), illusion.Fn2(reset), illusion.Fn4(present), illusion.Fn3(chase)),
		).
		AddSystems(illusion.Render, illusion.Fn2(hud).InSet(render.Draw2D)).
		Run()
}

func setup(
	cmd *illusion.Commands,
	materials *illusion.Res[asset.Assets[render.StandardMaterial]],
	meshes *illusion.Res[asset.Assets[render.Mesh]],
) {
	paint := func(c rl.Color) render.MeshMaterial3d {
		return render.MeshMaterial3d{Material: materials.Get().Add(render.StandardMaterial{BaseColor: c})}
	}
	box := func(w, h, l float32, c rl.Color, tr transform.Transform, extra ...illusion.Component) {
		cmd.Spawn(append([]illusion.Component{
			illusion.C(render.Mesh3d{Mesh: meshes.Get().Add(render.Cuboid(w, h, l))}),
			illusion.C(paint(c)), illusion.C(tr),
			illusion.C(physics.Static), illusion.C(physics.Cuboid(w, h, l)),
		}, extra...)...)
	}
	tilt := func(tr transform.Transform, degrees float32) transform.Transform {
		return tr.WithRotation(rl.QuaternionFromAxisAngle(transform.Right, degrees*rl.Deg2rad))
	}

	cmd.Spawn(
		illusion.C(render.DirectionalLight{Color: rl.NewColor(255, 246, 225, 255)}),
		illusion.C(transform.Identity().LookingAt(rl.Vector3{X: -0.5, Y: -1, Z: -0.3}, transform.Up)),
	)
	cmd.Spawn(
		illusion.C(Chase{}), illusion.C(render.Camera3d{Fovy: 60}),
		illusion.C(transform.FromXYZ(0, 4, -9).LookingAt(start.Translation, transform.Up)),
	)

	// The ground, 400 m across, and the track on it.
	box(400, 1, 400, rl.NewColor(120, 140, 110, 255), transform.FromXYZ(0, -0.5, 0),
		illusion.C(physics.Material{Friction: 0.9}))
	box(8, 0.4, 14, rl.NewColor(200, 150, 90, 255), tilt(transform.FromXYZ(0, 0.9, 30), -12))
	box(8, 2.3, 6, rl.NewColor(200, 150, 90, 255), transform.FromXYZ(0, 1.15, 39.8))
	box(8, 0.4, 14, rl.NewColor(200, 150, 90, 255), tilt(transform.FromXYZ(0, 0.9, 49.6), 12))
	for i := range 12 {
		box(10, 0.16, 0.5, rl.NewColor(90, 90, 90, 255), transform.FromXYZ(16, 0.04, float32(10+i*2)))
	}
	for i := range 8 {
		x := float32(-16)
		if i%2 == 1 {
			x = -12
		}
		box(0.5, 1, 0.5, rl.Orange, transform.FromXYZ(x, 0.5, float32(12+i*8)))
	}

	// The car: the body, then what's drawn of it.
	var wheels []physics.Wheel
	for _, z := range []float32{1.35, -1.35} {
		for _, x := range []float32{0.95, -0.95} {
			w := physics.Wheel{
				Position: rl.Vector3{X: x, Y: -0.05, Z: z}, Radius: wheelRadius, Width: wheelWidth,
				SuspensionMin: 0.15, SuspensionMax: 0.45, Frequency: 1.6, Damping: 0.5,
			}
			if z > 0 {
				w.MaxSteer = 0.55
			} else {
				w.HandBrake = 4000
			}
			wheels = append(wheels, w)
		}
	}
	shell := meshes.Get().Add(render.Cuboid(1.7, 0.6, 3.9))
	cabin := meshes.Get().Add(render.Cuboid(1.5, 0.5, 1.8))
	tyre := meshes.Get().Add(render.Cylinder(wheelRadius, wheelWidth))
	car := cmd.Spawn(
		illusion.C(Car{}), illusion.C(start),
		illusion.C(physics.Dynamic),
		illusion.C(physics.Cuboid(1.7, 0.6, 3.9).WithCenterOfMass(rl.Vector3{Y: -0.35})),
		illusion.C(physics.Mass(1300)),
		illusion.C(physics.Vehicle{
			Wheels:        wheels,
			Engine:        physics.Engine{MaxTorque: 600},
			Differentials: []physics.Differential{{Left: 0, Right: 1, TorqueRatio: 0.4}, {Left: 2, Right: 3, TorqueRatio: 0.6}},
			AntiRollBars:  []physics.AntiRollBar{{Left: 0, Right: 1, Stiffness: 2000}, {Left: 2, Right: 3, Stiffness: 2000}},
			MaxPitchRoll:  60 * rl.Deg2rad,
		}),
		illusion.C(physics.VehicleInput{}), illusion.C(physics.VehicleState{}), illusion.C(physics.Interpolated{}),
	)
	car.WithChildren(func(c *illusion.ChildBuilder) {
		body := c.Spawn(illusion.C(Shell{}), illusion.C(transform.Identity()),
			illusion.C(render.Mesh3d{Mesh: shell}), illusion.C(paint(rl.NewColor(214, 90, 40, 255))))
		body.WithChild(illusion.C(render.Mesh3d{Mesh: cabin}), illusion.C(paint(rl.NewColor(40, 44, 52, 255))),
			illusion.C(transform.FromXYZ(0, 0.55, -0.3)))
		body.WithChildren(func(c *illusion.ChildBuilder) {
			for i, w := range wheels {
				// The cylinder stands on its base along Y; lay it on its
				// side, centred, so its axle is the wheel model's X axis.
				model := transform.FromXYZ(-wheelWidth/2, 0, 0).
					WithRotation(rl.QuaternionFromAxisAngle(rl.Vector3{Z: 1}, -math.Pi/2))
				c.Spawn(illusion.C(WheelOf{Index: i}), illusion.C(transform.FromTranslation(w.Position))).
					WithChild(illusion.C(render.Mesh3d{Mesh: tyre}), illusion.C(paint(rl.NewColor(30, 30, 30, 255))), illusion.C(model))
			}
		})
	})
}

// drive turns the keys into the car's input.
func drive(cars *illusion.Query1Where[physics.VehicleInput, illusion.With[Car]], keys *illusion.Res[input.Keys], t *illusion.Res[illusion.Time]) {
	k := keys.Get()
	cars.Each(func(_ ecs.Entity, in *physics.VehicleInput) {
		var forward, right float32
		if k.AnyPressed(rl.KeyW, rl.KeyUp) {
			forward = 1
		}
		if k.AnyPressed(rl.KeyS, rl.KeyDown) {
			forward = -1
		}
		if k.AnyPressed(rl.KeyD, rl.KeyRight) {
			right++
		}
		if k.AnyPressed(rl.KeyA, rl.KeyLeft) {
			right--
		}
		// Ease the wheel toward full lock rather than snapping it there.
		step := 3 * t.Get().DeltaSecs()
		in.Right += max(-step, min(step, right-in.Right))
		in.Forward = forward
		in.HandBrake = 0
		if k.Pressed(rl.KeySpace) {
			in.HandBrake = 1
		}
	})
}

// reset puts the car back at the start, upright, when R is pressed.
func reset(cars *illusion.Query2Where[transform.Transform, physics.Velocity, illusion.With[Car]], keys *illusion.Res[input.Keys]) {
	if !keys.Get().JustPressed(rl.KeyR) {
		return
	}
	cars.Each(func(_ ecs.Entity, tr *transform.Transform, v *physics.Velocity) {
		*tr = start
		*v = physics.Velocity{}
	})
}

// present poses the car's shell between its last two steps, and its wheels.
func present(
	cars *illusion.Query3Where[transform.Transform, physics.Interpolated, physics.VehicleState, illusion.With[Car]],
	shells *illusion.Query1Where[transform.Transform, illusion.With[Shell]],
	wheels *illusion.Query2[WheelOf, transform.Transform],
	fixed *illusion.Res[illusion.FixedTime],
) {
	_, root, interp, state, ok := cars.Single()
	if !ok {
		return
	}
	pose := interp.At(fixed.Get().Overstep())
	inverse := rl.QuaternionInvert(root.Rotation)
	shells.Each(func(_ ecs.Entity, tr *transform.Transform) {
		tr.Translation = rl.Vector3RotateByQuaternion(rl.Vector3Subtract(pose.Translation, root.Translation), inverse)
		tr.Rotation = rl.QuaternionMultiply(inverse, pose.Rotation)
	})
	wheels.Each(func(_ ecs.Entity, w *WheelOf, tr *transform.Transform) {
		if w.Index < len(state.Wheels) {
			*tr = state.Wheels[w.Index].Transform
		}
	})
}

// chase follows the car from behind and above.
func chase(
	cameras *illusion.Query1Where[transform.Transform, illusion.With[Chase]],
	shells *illusion.Query1Where[transform.GlobalTransform, illusion.With[Shell]],
	t *illusion.Res[illusion.Time],
) {
	_, car, ok := shells.Single()
	if !ok {
		return
	}
	at := car.Translation()
	forward := car.Forward()
	forward.Y = 0
	if rl.Vector3LengthSqr(forward) < 1e-4 {
		forward = rl.Vector3{Z: 1}
	}
	forward = rl.Vector3Normalize(forward)
	eye := rl.Vector3Add(rl.Vector3Subtract(at, rl.Vector3Scale(forward, 8)), rl.Vector3{Y: 3})
	ease := 1 - float32(math.Exp(float64(-6*t.Get().DeltaSecs())))
	cameras.Each(func(_ ecs.Entity, tr *transform.Transform) {
		tr.Translation = rl.Vector3Lerp(tr.Translation, eye, ease)
		tr.LookAt(rl.Vector3Add(at, rl.Vector3{Y: 1}), transform.Up)
	})
}

func hud(cars *illusion.Query1Where[physics.VehicleState, illusion.With[Car]], win *illusion.Res[window.Window]) {
	_, st, ok := cars.Single()
	if !ok {
		return
	}
	gear := fmt.Sprint(st.Gear)
	switch {
	case st.Gear < 0:
		gear = "R"
	case st.Gear == 0:
		gear = "N"
	}
	rl.DrawText(fmt.Sprintf("%3.0f km/h   gear %s   %4.0f rpm", math.Abs(float64(st.Speed))*3.6, gear, st.RPM), 16, 16, 28, rl.White)
	rl.DrawText("W/S throttle and brake   A/D steer   space hand brake   R reset", 16, 50, 20, rl.White)
	rl.DrawFPS(int32(win.Get().Width)-90, 12)
}
