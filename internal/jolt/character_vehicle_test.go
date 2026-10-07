package jolt

import (
	"fmt"
	"math"
	"testing"
)

// The role must work in either body-ID order, for discrete contacts and CCD,
// without depending on the victim being lighter than the vehicle.
func TestCharacterBodyYieldsToVehicle(t *testing.T) {
	for _, first := range []bool{false, true} {
		for _, continuous := range []bool{false, true} {
			t.Run(fmt.Sprintf("character-first=%t/CCD=%t", first, continuous), func(t *testing.T) {
				w, _ := newVehicleWorld(t)
				w.SetGravity([3]float32{})
				var person BodyID
				spawn := func() {
					shape, err := NewBox([3]float32{.3, .4, .3})
					if err != nil {
						t.Fatal(err)
					}
					defer shape.Release()
					s := DefaultBodySettings(shape)
					s.Character, s.Continuous = true, continuous
					s.Mass, s.LinearDamping, s.AngularDamping = 7000, 0, 0
					s.Transform.Position = [3]float32{.7, 1.25, 2.6}
					person = w.CreateBody(s)
				}
				if first {
					spawn()
				}
				car := newCar(t, w, Wheeled, continuous)
				if !first {
					spawn()
				}
				w.SetVelocity(car, [3]float32{0, 0, 2}, [3]float32{})
				for frame := range 90 {
					if err := w.Step(1./60, 1); err != nil {
						t.Fatal(err)
					}
					linear, angular := w.Velocity(car)
					if math.Abs(float64(linear[0])) > .001 || math.Abs(float64(linear[1])) > .001 || math.Abs(float64(linear[2]-2)) > .2 {
						t.Fatalf("character changed vehicle momentum at frame %d: %v", frame, linear)
					}
					for _, spin := range angular {
						if math.IsNaN(float64(spin)) || math.Abs(float64(spin)) > .001 {
							t.Fatalf("character torqued vehicle at frame %d: %v", frame, angular)
						}
					}
				}
				if w.Transform(person).Position[2] < 3.6 {
					t.Fatal("vehicle did not push character")
				}
			})
		}
	}
}

// Wheel casts must reach the floor through a character; otherwise suspension
// forces can lift and flip a vehicle even when chassis contacts are harmless.
func TestVehicleWheelsIgnoreCharacters(t *testing.T) {
	for _, tester := range []int32{TestRay, TestSphere, TestCylinder} {
		t.Run(fmt.Sprint(tester), func(t *testing.T) {
			w, _ := newVehicleWorld(t)
			car := newCar(t, w, Wheeled)
			w.DestroyVehicle(car)
			vs := DefaultVehicleSettings()
			vs.Tester = tester
			wheel := DefaultWheelSettings()
			wheel.Position = [3]float32{0, -.2, 0}
			if !w.CreateVehicle(car, vs, []WheelSettings{wheel}, nil, nil) {
				t.Fatal("CreateVehicle failed")
			}
			shape, err := NewSphere(.25)
			if err != nil {
				t.Fatal(err)
			}
			defer shape.Release()
			s := DefaultBodySettings(shape)
			s.Character, s.Motion = true, Kinematic
			s.Transform.Position = [3]float32{0, .25, 0}
			w.CreateBody(s)
			step(w, 2)
			if y := w.Transform(car).Position[1]; y > .9 {
				t.Fatalf("wheel used character as ground: car y=%g", y)
			}
		})
	}
}
