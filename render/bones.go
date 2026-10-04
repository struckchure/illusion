package render

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/transform"
)

// BoneAttachment keeps an entity on a bone of its parent's Model3d, following
// the parent's AnimationPlayer (or the bind pose without one): every frame,
// before transforms propagate, it sets the entity's Transform to the bone's
// pose, then Offset. Spawn it as a child of the model's entity:
//
//	cmd.Spawn(illusion.C(render.Model3d{Model: knight}), illusion.C(player), ...).
//		WithChildren(func(b *illusion.ChildBuilder) {
//			b.Spawn(illusion.C(render.Model3d{Model: sword}), illusion.C(transform.Identity()),
//				illusion.C(render.BoneAttachment{Bone: "hand.R"}))
//		})
//
// To read where a bone is (to spawn a projectile from a hand, say), attach an
// empty entity and read its GlobalTransform.
type BoneAttachment struct {
	// Bone is the bone's name in the model file.
	Bone string
	// Offset places the entity relative to the bone. The zero value means
	// none.
	Offset transform.Transform

	index    int          // cached index of Bone in the skeleton
	resolved string       // the Bone that index is for
	skeleton *rl.BoneInfo // the skeleton index is for
}

// attachToBones moves bone attachments to their bones' current poses.
func attachToBones(
	q *illusion.Query2[BoneAttachment, transform.Transform],
	h *illusion.Hierarchy,
	models *illusion.Query1[Model3d],
	players *illusion.Query1[AnimationPlayer],
	modelStore *illusion.Res[asset.Assets[Model]],
	animStore *illusion.Res[asset.Assets[Animations]],
) {
	q.Each(func(e ecs.Entity, at *BoneAttachment, tr *transform.Transform) {
		parent, ok := h.Parent(e)
		if !ok {
			return
		}
		m3d, ok := models.Get(parent)
		if !ok {
			return
		}
		model := modelStore.Get().Get(m3d.Model)
		if model == nil || !at.resolve(&model.Model) {
			return
		}
		pose := model.Skeleton.GetBindPose()[at.index]
		if p, ok := players.Get(parent); ok {
			if a := animStore.Get().Get(p.Animations); a != nil {
				if animated, ok := bonePose(&model.Model, p, a, at.index); ok {
					pose = animated
				}
			}
		}
		m := rl.MatrixMultiply(poseMatrix(pose), model.Transform)
		if at.Offset != (transform.Transform{}) {
			m = rl.MatrixMultiply(at.Offset.Matrix(), m)
		}
		*tr = transform.GlobalTransform{Matrix: m}.Decompose()
	})
}

// resolve finds the attachment's bone in the model's skeleton.
func (at *BoneAttachment) resolve(model *rl.Model) bool {
	bones := unsafeBones(model)
	if at.resolved == at.Bone && at.skeleton == model.Skeleton.Bones && at.index < len(bones) {
		return true
	}
	for i, b := range bones {
		if boneName(b.Name) == at.Bone {
			at.index, at.resolved, at.skeleton = i, at.Bone, model.Skeleton.Bones
			return true
		}
	}
	return false
}

func unsafeBones(model *rl.Model) []rl.BoneInfo {
	if model.Skeleton.Bones == nil || model.Skeleton.BoneCount <= 0 {
		return nil
	}
	return model.Skeleton.GetBones()
}

func boneName(name [32]int8) string {
	b := make([]byte, 0, len(name))
	for _, c := range name {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b)
}

// bonePose computes a bone's model-space pose for the player's current
// state, the way poseModel's raylib calls do, so attachments line up with the
// drawn mesh.
func bonePose(model *rl.Model, p *AnimationPlayer, a *Animations, bone int) (rl.Transform, bool) {
	if len(p.Pose) == int(model.Skeleton.BoneCount) && bone >= 0 && bone < len(p.Pose) {
		return p.Pose[bone], true
	}
	return clipBonePose(model, p, a, bone)
}

// SamplePose samples the clips and their current crossfade, ignoring Pose.
// Reuse dst between frames, then modify the result and assign it to Pose.
func (p *AnimationPlayer) SamplePose(model *rl.Model, a *Animations, dst []rl.Transform) []rl.Transform {
	cur, ok := a.Clip(p.current.clip)
	if !ok || !a.fits(model, cur) {
		return nil
	}
	n := int(model.Skeleton.BoneCount)
	if cap(dst) < n {
		dst = make([]rl.Transform, n)
	} else {
		dst = dst[:n]
	}
	for i := range dst {
		dst[i], _ = clipBonePose(model, p, a, i)
	}
	return dst
}

func clipBonePose(model *rl.Model, p *AnimationPlayer, a *Animations, bone int) (rl.Transform, bool) {
	cur, ok := a.Clip(p.current.clip)
	if !ok || !a.fits(model, cur) {
		return rl.Transform{}, false
	}
	frame := a.frame(cur, p.current.time, p.current.once)
	if p.fadeLength > 0 {
		if prev, ok := a.Clip(p.previous.clip); ok && a.fits(model, prev) {
			// As UpdateModelAnimationEx: blend 0 is the previous clip.
			from := sample(a.Clips[prev], a.frame(prev, p.previous.time, p.previous.once), bone, true)
			to := sample(a.Clips[cur], frame, bone, true)
			return mix(from, to, min(p.fade/p.fadeLength, 1)), true
		}
	}
	return sample(a.Clips[cur], frame, bone, false), true
}

// sample interpolates a bone's pose at a fractional frame. raylib's
// UpdateModelAnimation splits the frame before wrapping it, while
// UpdateModelAnimationEx (ex) wraps first; with frames inside the clip, as
// here, both agree.
func sample(anim rl.ModelAnimation, frame float32, bone int, ex bool) rl.Transform {
	n := int(anim.KeyframeCount)
	current := int(frame)
	if ex {
		current %= n
	}
	blend := max(0, min(1, frame-float32(current)))
	current %= n
	next := (current + 1) % n
	return mix(anim.GetFramePose(current, bone), anim.GetFramePose(next, bone), blend)
}

func mix(a, b rl.Transform, t float32) rl.Transform {
	return rl.Transform{
		Translation: rl.Vector3Lerp(a.Translation, b.Translation, t),
		Rotation:    rl.QuaternionSlerp(a.Rotation, b.Rotation, t),
		Scale:       rl.Vector3Lerp(a.Scale, b.Scale, t),
	}
}

// poseMatrix is a pose as raylib builds bone matrices: scale, rotate, move.
func poseMatrix(t rl.Transform) rl.Matrix {
	return rl.MatrixMultiply(
		rl.MatrixMultiply(rl.MatrixScale(t.Scale.X, t.Scale.Y, t.Scale.Z), rl.QuaternionToMatrix(t.Rotation)),
		rl.MatrixTranslate(t.Translation.X, t.Translation.Y, t.Translation.Z))
}
