package jolt

import (
	"math"
	"testing"
)

// newCar puts a 1500 kg car (or, with Motorcycle, a 2-wheeled bike) on the
// test world's ground, facing +Z.
func newCar(t *testing.T, w *World, controller int32, continuous ...bool) BodyID {
	t.Helper()
	halfWidth, halfLength := float32(0.9), float32(2)
	if controller == Motorcycle {
		halfWidth, halfLength = 0.2, 0.8
	}
	box, err := NewBox([3]float32{halfWidth, 0.3, halfLength})
	if err != nil {
		t.Fatal(err)
	}
	defer box.Release()
	low, err := box.OffsetCenterOfMass([3]float32{0, -0.3, 0})
	if err != nil {
		t.Fatal(err)
	}
	defer low.Release()

	s := DefaultBodySettings(low)
	s.Transform.Position = [3]float32{0, 1, 0}
	s.Mass = 1500
	if len(continuous) > 0 {
		s.Continuous = continuous[0]
	}
	if controller == Motorcycle {
		s.Mass = 240
	}
	body := w.CreateBody(s)

	vs := DefaultVehicleSettings()
	vs.Controller = controller
	var wheels []WheelSettings
	var diffs []Differential
	if controller == Motorcycle {
		for _, z := range []float32{halfLength, -halfLength} {
			ws := DefaultWheelSettings()
			ws.Position = [3]float32{0, -0.2, z}
			ws.Radius, ws.Width = 0.31, 0.05
			if z < 0 {
				ws.MaxSteer = 0
			} else {
				ws.MaxSteer = 0.5
			}
			wheels = append(wheels, ws)
		}
		vs.MaxTorque = 150
		d := DefaultDifferential()
		d.Left, d.Right, d.Ratio = -1, 1, 1.93*40/16
		diffs = []Differential{d}
	} else {
		for _, z := range []float32{halfLength - 0.6, -halfLength + 0.6} {
			for _, x := range []float32{halfWidth, -halfWidth} {
				ws := DefaultWheelSettings()
				ws.Position = [3]float32{x, -0.2, z}
				if z < 0 {
					ws.MaxSteer = 0
				} else {
					ws.MaxSteer = 0.5
				}
				wheels = append(wheels, ws)
			}
		}
		front, rear := DefaultDifferential(), DefaultDifferential()
		front.Left, front.Right, front.TorqueRatio = 0, 1, 0.5
		rear.Left, rear.Right, rear.TorqueRatio = 2, 3, 0.5
		diffs = []Differential{front, rear}
	}
	bars := []AntiRollBar{{Left: 0, Right: 1, Stiffness: 1000}}
	if controller == Motorcycle {
		bars = nil
	}
	if !w.CreateVehicle(body, vs, wheels, diffs, bars) {
		t.Fatal("CreateVehicle failed")
	}
	return body
}

// newVehicleWorld is newTestWorld with a ground a kilometre across, room to
// drive.
func newVehicleWorld(t *testing.T) (*World, BodyID) {
	t.Helper()
	w := NewWorld(1024)
	t.Cleanup(w.Close)
	w.SetGravity([3]float32{0, -9.81, 0})
	ground, err := NewBox([3]float32{500, 0.5, 500})
	if err != nil {
		t.Fatal(err)
	}
	defer ground.Release()
	s := DefaultBodySettings(ground)
	s.Motion = Static
	s.Transform.Position = [3]float32{0, -0.5, 0}
	return w, w.CreateBody(s)
}

func drive(w *World, body BodyID, seconds, forward, right, brake float32) {
	for range int(seconds * 60) {
		w.SetVehicleInput(body, forward, right, brake, 0)
		w.Step(1.0/60, 1)
	}
}

func yaw(q [4]float32) float64 {
	// Heading of +Z after rotation.
	x, y, z, ww := float64(q[0]), float64(q[1]), float64(q[2]), float64(q[3])
	fx := 2 * (x*z + ww*y)
	fz := 1 - 2*(x*x+y*y)
	return math.Atan2(fx, fz)
}

func TestVehicleSettlesOnItsSuspension(t *testing.T) {
	w, _ := newVehicleWorld(t)
	car := newCar(t, w, Wheeled)
	step(w, 2)

	wheels := w.VehicleWheels(car, nil)
	if len(wheels) != 4 {
		t.Fatalf("got %d wheels, want 4", len(wheels))
	}
	for i, wh := range wheels {
		if !wh.Contact {
			t.Errorf("wheel %d isn't on the ground", i)
		}
		if wh.Suspension <= 0.3 || wh.Suspension >= 0.5 {
			t.Errorf("wheel %d suspension %v should be between its limits", i, wh.Suspension)
		}
		// The wheel's center sits a radius above the ground.
		y := w.Transform(car).Position[1] + wh.Transform.Position[1]
		if math.Abs(float64(y-0.3)) > 0.05 {
			t.Errorf("wheel %d center at y=%v, want about its radius 0.3", i, y)
		}
	}
	if s := w.VehicleStatus(car); s.Touching != 4 {
		t.Fatalf("status says %d wheels touch, want 4", s.Touching)
	}
}

func TestVehicleDrivesSteersAndBrakes(t *testing.T) {
	w, _ := newVehicleWorld(t)
	car := newCar(t, w, Wheeled)
	step(w, 1)

	drive(w, car, 3, 1, 0, 0)
	p := w.Transform(car).Position
	if p[2] < 5 {
		t.Fatalf("car should have driven forward along +Z, got %v", p)
	}
	st := w.VehicleStatus(car)
	if st.Speed < 3 || st.Gear < 1 {
		t.Fatalf("car should be moving forward in gear, got %+v", st)
	}
	if spin := w.VehicleWheels(car, nil)[0].Spin; spin <= 0 {
		t.Fatalf("wheels should spin forward, got %v rad/s", spin)
	}

	before := yaw(w.Transform(car).Rotation)
	drive(w, car, 1, 1, 1, 0)
	if steer := w.VehicleWheels(car, nil)[0].Steer; steer == 0 {
		t.Fatal("front wheels should steer")
	}
	after := yaw(w.Transform(car).Rotation)
	if math.Abs(after-before) < 0.1 {
		t.Fatalf("steering should turn the car, yaw %v -> %v", before, after)
	}

	drive(w, car, 4, 0, 0, 1)
	if s := w.VehicleStatus(car).Speed; math.Abs(float64(s)) > 0.5 {
		t.Fatalf("braking should stop the car, still at %v m/s", s)
	}
}

func TestMotorcycleStaysUp(t *testing.T) {
	w, _ := newVehicleWorld(t)
	bike := newCar(t, w, Motorcycle)
	step(w, 1)
	drive(w, bike, 4, 1, 0, 0)
	q := w.Transform(bike).Rotation
	x, z := float64(q[0]), float64(q[2])
	upY := 1 - 2*(x*x+z*z)
	if upY < 0.7 {
		t.Fatalf("bike fell over: up·Y = %v", upY)
	}
	if p := w.Transform(bike).Position; p[2] < 3 {
		t.Fatalf("bike should have ridden forward, got %v", p)
	}
}

func TestVehicleNeedsADynamicBody(t *testing.T) {
	w, ground := newTestWorld(t)
	if w.CreateVehicle(ground, DefaultVehicleSettings(), []WheelSettings{DefaultWheelSettings()}, nil, nil) {
		t.Fatal("a static body shouldn't take a vehicle")
	}
	car := newCar(t, w, Wheeled)
	if w.CreateVehicle(car, DefaultVehicleSettings(), []WheelSettings{DefaultWheelSettings()}, nil, nil) {
		t.Fatal("a body takes one vehicle")
	}
	w.DestroyVehicle(car)
	if n := len(w.VehicleWheels(car, nil)); n != 0 {
		t.Fatalf("destroyed vehicle still has %d wheels", n)
	}
	if w.VehicleStatus(car) != (VehicleStatus{}) {
		t.Fatal("destroyed vehicle still reports status")
	}
}

func TestDestroyingABodyOrWorldFreesItsVehicle(t *testing.T) {
	w, _ := newVehicleWorld(t)
	car := newCar(t, w, Wheeled)
	step(w, 0.5)
	w.DestroyBody(car)
	step(w, 0.5) // a stepped constraint on a freed body would crash
	w.SetVehicleInput(car, 1, 0, 0, 0)

	newCar(t, w, Wheeled)
	step(w, 0.5)
	w.Close() // with a vehicle still in it
}

func TestCenterOfMassOffsetRightsATallBox(t *testing.T) {
	w, _ := newTestWorld(t)
	box, _ := NewBox([3]float32{0.3, 1, 0.3})
	defer box.Release()
	low, _ := box.OffsetCenterOfMass([3]float32{0, -0.9, 0})
	defer low.Release()
	s := DefaultBodySettings(low)
	// Tipped 30° onto one edge: weighted at the bottom, it rocks back up.
	half := math.Pi / 12
	s.Transform = Transform{Position: [3]float32{0, 1.2, 0}, Rotation: [4]float32{0, 0, float32(math.Sin(half)), float32(math.Cos(half))}}
	id := w.CreateBody(s)
	step(w, 4)
	q := w.Transform(id).Rotation
	if math.Abs(float64(q[2])) > 0.05 {
		t.Fatalf("a bottom-weighted box should stand back up, got rotation %v", q)
	}
}
