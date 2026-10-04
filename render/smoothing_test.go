package render

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"math"
	"testing"
	"time"
)

func TestClothPresentationBetweenSteps(t *testing.T) {
	v, tri := strip(2)
	c := newClothMesh(v, tri, ClothFreedom(v, tri, []bool{true, true}, 1, 1))
	p := c.particle[4]
	c.target[p] = rl.Vector3{X: 2}
	c.drawPrev[p] = rl.Vector3{Z: 0.02}
	c.drawOffset[p] = rl.Vector3{Z: 0.06}
	for _, alpha := range []float32{0, 0.25, 0.5, 0.75, 1} {
		got := c.drawAt(p, alpha, nil)
		if math.Abs(float64(got.Z-(0.02+0.04*alpha))) > 1e-5 || got.X != 2 {
			t.Fatalf("alpha %v: %v; expected interpolated displacement on current pose", alpha, got)
		}
	}
	c.target[p].X = 3 // no physics tick: still follows the latest body pose
	if got := c.drawAt(p, 0.5, nil); got.X != 3 {
		t.Fatalf("cloth left behind between ticks: %v", got)
	}
}

func TestClothScheduleSamplesPhysicsTime(t *testing.T) {
	s := &clothState{}
	f := s.schedule(clothStep / 2)
	if f.steps != 0 || math.Abs(float64(f.alpha-0.5)) > 1e-5 {
		t.Fatalf("first half frame: %+v", f)
	}
	f = s.schedule(clothStep)
	if f.steps != 1 || math.Abs(float64(f.first-0.5)) > 1e-5 {
		t.Fatalf("tick should sample halfway through frame: %+v", f)
	}
	f = s.schedule(clothStep / 2)
	if f.steps != 1 || math.Abs(float64(f.first-1)) > 1e-5 {
		t.Fatalf("tick at frame end: %+v", f)
	}
	f = s.schedule(1)
	if f.steps != clothMaxSteps || f.first < 0 || f.first > 1 || f.alpha < 0 || f.alpha >= 1 {
		t.Fatalf("long frame: %+v", f)
	}
}

func TestClothNormalsFollowDeformationAcrossSeams(t *testing.T) {
	v, tri := strip(2)
	c := newClothMesh(v, tri, ClothFreedom(v, tri, []bool{true, true}, 1, 1))
	posed := append([]rl.Vector3(nil), v...)
	normals := make([]rl.Vector3, len(v))
	for i := range normals {
		normals[i] = rl.Vector3{Z: 1}
	}
	c.surfaceNormals(c.poseNormal, posed)
	for i := range posed {
		posed[i].Z = -posed[i].Y
	}
	c.deformNormals(posed, normals)
	p := 4
	want := rl.Vector3Normalize(rl.Vector3{Y: 1, Z: 1})
	if rl.Vector3Distance(normals[p], want) > 1e-4 {
		t.Fatalf("fold normal = %v, want %v", normals[p], want)
	}
	if normals[p] != normals[len(v)-2] {
		t.Fatalf("UV seam normals differ: %v / %v", normals[p], normals[len(v)-2])
	}
	if normals[0] != (rl.Vector3{Z: 1}) {
		t.Fatalf("authored pinned normal changed: %v", normals[0])
	}
}

func TestAnimationFadeUsesWallTimeAtDifferentSpeeds(t *testing.T) {
	for _, speed := range []float32{0.6, 1, 1.4, -1} {
		app, e, _ := newAnimApp(t, func(p *AnimationPlayer) {
			p.Play("Walk")
			p.Play("Jump").FadeIn(0.2)
			p.Speed = speed
		})
		app.Tick(100 * time.Millisecond)
		p := player(app, e)
		if math.Abs(float64(p.fade-0.1)) > 1e-5 {
			t.Fatalf("speed %v: fade = %v, want 0.1", speed, p.fade)
		}
		app.Tick(110 * time.Millisecond)
		if p.fadeLength != 0 {
			t.Fatalf("speed %v: fade did not complete", speed)
		}
	}
}
