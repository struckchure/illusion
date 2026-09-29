package render

import (
	"math"
	"testing"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/transform"
)

// A two-bone skeleton in model space: Root at the origin and Tip one unit
// up, in a model shifted two units along X.
func testSkeleton() rl.Model {
	var root, tip rl.BoneInfo
	for i, c := range "Root" {
		root.Name[i] = int8(c)
	}
	for i, c := range "Tip" {
		tip.Name[i] = int8(c)
	}
	tip.Parent = 0
	bones := []rl.BoneInfo{root, tip}
	bind := []rl.Transform{
		{Rotation: rl.QuaternionIdentity(), Scale: rl.Vector3One()},
		{Translation: rl.Vector3{Y: 1}, Rotation: rl.QuaternionIdentity(), Scale: rl.Vector3One()},
	}
	return rl.Model{
		Transform: rl.MatrixTranslate(2, 0, 0),
		Skeleton:  rl.ModelSkeleton{BoneCount: 2, Bones: &bones[0], BindPose: &bind[0]},
	}
}

// poseClip makes a clip whose Tip rotates about Z through the given angles,
// one keyframe each (Root stays put).
func poseClip(name string, degrees ...float32) rl.ModelAnimation {
	frames := make([]rl.ModelAnimPose, len(degrees))
	for i, d := range degrees {
		pose := []rl.Transform{
			{Rotation: rl.QuaternionIdentity(), Scale: rl.Vector3One()},
			{Translation: rl.Vector3{Y: 1}, Rotation: rl.QuaternionFromAxisAngle(rl.Vector3{Z: 1}, d*math.Pi/180), Scale: rl.Vector3One()},
		}
		frames[i] = &pose[0]
	}
	a := clip(name, int32(len(degrees)))
	a.BoneCount = 2
	a.KeyframePoses = &frames[0]
	return a
}

// newBoneApp spawns the model with an optional player and a child attached to
// bone with offset, and returns the app and the child.
func newBoneApp(t *testing.T, bone string, offset transform.Transform, play func(p *AnimationPlayer)) (*illusion.App, ecs.Entity) {
	t.Helper()
	app := illusion.New()
	asset.Register[Model](app, nil) // no unload: the test model is Go memory
	buildAnimation(app)
	app.AddSystems(illusion.Startup, illusion.Fn3(func(
		cmd *illusion.Commands,
		models *illusion.Res[asset.Assets[Model]],
		anims *illusion.Res[asset.Assets[Animations]],
	) {
		parent := []illusion.Component{
			illusion.C(Model3d{Model: models.Get().Add(Model{Model: testSkeleton()})}),
			illusion.C(transform.Identity()),
		}
		if play != nil {
			p := AnimationPlayer{Animations: anims.Get().Add(Animations{Clips: []rl.ModelAnimation{
				poseClip("Bend", 0, 90, 0), poseClip("Rest", 0, 0), poseClip("Up", 90, 90),
			}})}
			play(&p)
			parent = append(parent, illusion.C(p))
		}
		cmd.Spawn(parent...).WithChildren(func(b *illusion.ChildBuilder) {
			b.Spawn(illusion.C(transform.Identity()), illusion.C(BoneAttachment{Bone: bone, Offset: offset}))
		})
	}))
	app.Tick(0)
	var child ecs.Entity
	query := ecs.NewFilter1[BoneAttachment](app.World).Query()
	for query.Next() {
		child = query.Entity()
	}
	return app, child
}

func localTransform(app *illusion.App, e ecs.Entity) transform.Transform {
	return *ecs.NewMap[transform.Transform](app.World).Get(e)
}

func nearVec(a, b rl.Vector3) bool {
	return rl.Vector3Distance(a, b) < 1e-3
}

// zAngle returns the rotation's angle about Z in degrees.
func zAngle(q rl.Quaternion) float32 {
	return float32(2 * math.Atan2(float64(q.Z), float64(q.W)) * 180 / math.Pi)
}

func TestBoneAttachmentBindPose(t *testing.T) {
	app, child := newBoneApp(t, "Tip", transform.Transform{}, nil)
	app.Tick(time.Second / 60)
	tr := localTransform(app, child)
	if !nearVec(tr.Translation, rl.Vector3{X: 2, Y: 1}) || !near(zAngle(tr.Rotation), 0) {
		t.Fatalf("without a player the tip should sit at its bind pose (2,1,0): %+v", tr)
	}
}

func TestBoneAttachmentFollowsAnimation(t *testing.T) {
	app, child := newBoneApp(t, "Tip", transform.FromXYZ(0, 0.5, 0), func(p *AnimationPlayer) { p.Play("Bend") })
	app.Tick(time.Second / 60) // keyframe 1: the tip turned 90° about Z
	tr := localTransform(app, child)
	// The offset is in bone space: half a unit along the tip's own +Y, which
	// the 90° turn points along -X.
	if !nearVec(tr.Translation, rl.Vector3{X: 1.5, Y: 1}) || !near(zAngle(tr.Rotation), 90) {
		t.Fatalf("at keyframe 1: %+v (angle %v), want (1.5,1,0) at 90°", tr, zAngle(tr.Rotation))
	}

	app.Tick(time.Second / 120) // halfway from keyframe 1 back to 0
	if a := zAngle(localTransform(app, child).Rotation); !near(a, 45) {
		t.Fatalf("halfway between keyframes: %v°, want 45°", a)
	}
}

func TestBoneAttachmentCrossfades(t *testing.T) {
	app, child := newBoneApp(t, "Tip", transform.Transform{}, func(p *AnimationPlayer) {
		p.Play("Rest")
		p.Play("Up").FadeIn(1)
	})
	for range 30 { // half a second of the one-second fade
		app.Tick(time.Second / 60)
	}
	if a := zAngle(localTransform(app, child).Rotation); !near(a, 45) {
		t.Fatalf("halfway through fading from 0° to 90°: %v°", a)
	}
}

func TestBoneAttachmentUnknownBone(t *testing.T) {
	app, child := newBoneApp(t, "Tail", transform.Transform{}, nil)
	app.Tick(time.Second / 60)
	if tr := localTransform(app, child); tr != transform.Identity() {
		t.Fatalf("an unknown bone should leave the transform alone: %+v", tr)
	}
}
