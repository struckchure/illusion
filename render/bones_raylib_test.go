//go:build !js

package render

import (
	"runtime"
	"testing"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// bonePose must agree with the pose raylib computes when drawing, or
// attachments drift off their bones. raylib's update needs no GPU for a model
// without meshes, so compare against it directly.
func TestBonePoseMatchesRaylib(t *testing.T) {
	model := testSkeleton()
	pose := make([]rl.Transform, 2)
	matrices := make([]rl.Matrix, 2)
	model.CurrentPose, model.BoneMatrices = &pose[0], &matrices[0]

	a := &Animations{Clips: []rl.ModelAnimation{
		poseClip("Bend", 0, 90, 0), poseClip("Wave", 30, -40, 70, 30),
	}}
	// The keyframe arrays are Go memory that raylib reads through pointers
	// held in Go memory, which cgo only allows once they're pinned.
	var pins runtime.Pinner
	defer pins.Unpin()
	for _, c := range a.Clips {
		for _, frame := range unsafe.Slice(c.KeyframePoses, c.KeyframeCount) {
			pins.Pin(frame)
		}
	}
	posed := map[*rl.Mesh]appliedPose{}
	var cloth clothState
	check := func(p *AnimationPlayer, label string) {
		t.Helper()
		poseModel(model, p, a, posed) // raylib writes model.CurrentPose and BoneMatrices
		// Cloth poses the model itself, and has to agree too.
		if !poseBones(&cloth, &model, p, a) {
			t.Fatalf("%s: poseBones failed", label)
		}
		for bone := range 2 {
			probe := rl.Vector3{X: 0.3, Y: 1.1, Z: -0.2}
			if got, want := rl.Vector3Transform(probe, cloth.bones[bone]), rl.Vector3Transform(probe, matrices[bone]); !nearVec(got, want) {
				t.Fatalf("%s bone %d: cloth's matrix moves a point to %v, raylib's to %v", label, bone, got, want)
			}
		}
		for bone := range 2 {
			got, ok := bonePose(&model, p, a, bone)
			want := pose[bone]
			if !ok || !nearVec(got.Translation, want.Translation) || !nearQuat(got.Rotation, want.Rotation) {
				t.Fatalf("%s bone %d: Go pose %+v, raylib %+v", label, bone, got, want)
			}
		}
	}

	p := &AnimationPlayer{}
	p.Play("Wave")
	for i := range 100 {
		p.current.time = float32(i) * 0.0007 // sweeps fractional frames and the wrap
		check(p, "looping")
	}
	p.Play("Bend").FadeIn(1)
	for i := range 100 {
		p.previous.time, p.current.time, p.fade = float32(i)*0.0009, float32(i)*0.0004, float32(i)*0.01
		check(p, "crossfading")
	}
}

func nearQuat(a, b rl.Quaternion) bool {
	d := a.X*b.X + a.Y*b.Y + a.Z*b.Z + a.W*b.W
	return d > 0.99999 || d < -0.99999 // same rotation (q and -q are equal)
}
