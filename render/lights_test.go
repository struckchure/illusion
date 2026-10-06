package render

import (
	"math"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func light(x, reach float32) pointLight {
	return pointLight{position: rl.Vector3{X: x}, color: rl.Vector3{X: 1, Y: 1, Z: 1}, reach: reach}
}

func TestSelectLightsKeepsNearestReach(t *testing.T) {
	lights := []pointLight{light(50, 5), light(10, 2), light(30, 40), light(5, 0)}
	got := selectLights(rl.Vector3{}, lights, 8)
	if len(got) != 3 {
		t.Fatalf("got %d lights; a light without reach should be left out", len(got))
	}
	// Scored by how near their reach comes: -10, 8, 45.
	for i, x := range []float32{30, 10, 50} {
		if got[i].position.X != x || got[i].color.X != 1 {
			t.Fatalf("light %d: %+v; want the one at %v, unfaded", i, got[i], x)
		}
	}
}

func TestSelectLightsFadesTheLastOutSmoothly(t *testing.T) {
	// Two lights compete for one place; as the camera moves from one to the
	// other, whichever is kept fades to nothing as they swap.
	lights := []pointLight{light(0, 5), light(20, 5)}
	brightness := func(x float32) float32 {
		got := selectLights(rl.Vector3{X: x}, lights, 1)
		return got[0].color.X
	}
	if b := brightness(0); b != 1 {
		t.Fatalf("at the first light: %v; want it full", b)
	}
	if b := brightness(10); b > 1e-5 {
		t.Fatalf("halfway, where they swap: %v; want nothing", b)
	}
	if b := brightness(8); math.Abs(float64(b-1)) > 1e-5 {
		t.Fatalf("4 m short of halfway: %v; want full", b)
	}
	if b := brightness(9); math.Abs(float64(b-0.5)) > 1e-5 {
		t.Fatalf("1 m short of halfway: %v; want half", b)
	}
}

func TestPlaceShadowSnapsToTexels(t *testing.T) {
	dir := rl.Vector3Normalize(rl.Vector3{X: -1, Y: -0.4, Z: 0.3})
	a := placeShadow(rl.Vector3{X: 3.01, Y: 2, Z: -7}, rl.Vector3{Z: -1}, dir, 50, 200, 2048)
	b := placeShadow(rl.Vector3{X: 3.02, Y: 2, Z: -7.005}, rl.Vector3{Z: -1}, dir, 50, 200, 2048)
	texel := float64(100) / 2048
	for _, v := range []shadowView{a, b} {
		for _, axis := range []rl.Vector3{v.right, v.up} {
			k := float64(rl.Vector3DotProduct(v.focus, axis)) / texel
			if math.Abs(k-math.Round(k)) > 1e-2 {
				t.Fatalf("focus %v is %v texels along %v; want whole texels", v.focus, k, axis)
			}
		}
	}
	if d := rl.Vector3Subtract(a.camera.Position, a.camera.Target); math.Abs(float64(rl.Vector3Length(d))-200) > 1e-3 ||
		rl.Vector3DotProduct(rl.Vector3Normalize(d), dir) > -0.999 {
		t.Fatalf("camera at %v looking at %v; want it 200 m back towards the light", a.camera.Position, a.camera.Target)
	}
	if a.camera.Fovy != 100 || a.camera.Projection != rl.CameraOrthographic {
		t.Fatalf("camera %+v; want an orthographic view 100 m wide", a.camera)
	}
}

func TestShadowViewSees(t *testing.T) {
	v := placeShadow(rl.Vector3{}, rl.Vector3{Z: -1}, rl.Vector3{Y: -1, X: 0.2}, 20, 80, 1024)
	if !v.sees(v.focus, 0) {
		t.Fatal("doesn't see its own middle")
	}
	if !v.sees(rl.Vector3Add(v.focus, rl.Vector3Scale(v.right, 25)), 6) {
		t.Fatal("doesn't see a sphere reaching into the square")
	}
	if v.sees(rl.Vector3Add(v.focus, rl.Vector3Scale(v.right, 30)), 6) {
		t.Fatal("sees a sphere outside the square")
	}
	// Along the light, everything is in it.
	if !v.sees(rl.Vector3Add(v.focus, rl.Vector3{Y: 500, X: -100}), 1) {
		t.Fatal("doesn't see something up towards the light")
	}
}
