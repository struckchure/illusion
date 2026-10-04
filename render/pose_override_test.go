package render

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"testing"
)

func TestPoseOverrideMatchesClothAndKeepsSamplingClips(t *testing.T) {
	model := testSkeleton()
	current := make([]rl.Transform, 2)
	matrices := make([]rl.Matrix, 2)
	model.CurrentPose, model.BoneMatrices = &current[0], &matrices[0]
	animations := &Animations{Clips: []rl.ModelAnimation{poseClip("rest", 0, 0), poseClip("bend", 90, 90)}}
	player := &AnimationPlayer{}
	player.Play("rest")
	player.Play("bend").FadeIn(1)
	player.fade = .5
	sampled := player.SamplePose(&model, animations, nil)
	if len(sampled) != 2 || abs32(zAngle(sampled[1].Rotation)-45) > .001 {
		t.Fatal("sample did not include crossfade")
	}
	player.Pose = append([]rl.Transform(nil), sampled...)
	player.Pose[1].Translation.X = .4
	player.Pose[1].Rotation = rl.QuaternionIdentity()
	authored := player.SamplePose(&model, animations, nil)
	if authored[1].Translation.X != 0 || abs32(zAngle(authored[1].Rotation)-45) > .001 {
		t.Fatal("pose correction accumulated into authored sampling")
	}
	if bone, ok := bonePose(&model, player, animations, 1); !ok || bone.Translation.X != .4 {
		t.Fatal("attachments ignore override")
	}
	cache := map[*rl.Mesh]appliedPose{}
	poseModel(model, player, animations, cache)
	var cloth clothState
	if !poseBones(&cloth, &model, player, animations) {
		t.Fatal("cloth pose unavailable")
	}
	for i := range 2 {
		if !nearVec(current[i].Translation, player.Pose[i].Translation) {
			t.Fatal("draw pose differs from corrected pose")
		}
		probe := rl.Vector3{X: .3, Y: .7, Z: -.4}
		if !nearVec(rl.Vector3Transform(probe, matrices[i]), rl.Vector3Transform(probe, cloth.bones[i])) {
			t.Fatal("clothing and body disagree")
		}
	}
	if !cache[model.Meshes].custom {
		t.Fatal("mutable override retained an ordinary pose cache")
	}
	player.Pose = nil
	if bone, _ := bonePose(&model, player, animations, 1); bone.Translation.X != 0 {
		t.Fatal("clearing override retained contact")
	}
}
func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
