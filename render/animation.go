package render

import (
	"cmp"
	"errors"
	"math"
	"slices"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/transform"
)

// Animations are the skeletal animation clips in a model file (.glb, .gltf,
// .iqm, .m3d). Load them with asset.Loader[Animations] from the same file as
// the Model, and play them with an [AnimationPlayer].
type Animations struct {
	Clips []rl.ModelAnimation
	// FrameRate is how many keyframes the clips have per second. It defaults
	// to 60, the rate raylib samples glTF animations at; set it to the export
	// rate for IQM and M3D files.
	FrameRate float32

	byName map[string]int
	ends   []float32 // the keyframe each clip holds once it's played through (see end)
}

// Clip returns the index of the clip called name.
func (a *Animations) Clip(name string) (int, bool) {
	if a.byName == nil {
		a.byName = make(map[string]int, len(a.Clips))
		for i := range a.Clips {
			a.byName[a.Clips[i].GetName()] = i
		}
	}
	i, ok := a.byName[name]
	return i, ok
}

// Names returns the clips' names, in file order.
func (a *Animations) Names() []string {
	names := make([]string, len(a.Clips))
	for i := range a.Clips {
		names[i] = a.Clips[i].GetName()
	}
	return names
}

// Duration returns the length of clip i in seconds.
func (a *Animations) Duration(i int) float32 {
	return float32(max(a.Clips[i].KeyframeCount-1, 0)) / a.frameRate()
}

func (a *Animations) frameRate() float32 {
	if a.FrameRate <= 0 {
		return 60
	}
	return a.FrameRate
}

// frame returns the (fractional) keyframe of clip i after t seconds. Looping
// clips wrap at their last keyframe, which is equal to the first, so raylib
// never interpolates from the last keyframe back to the first. Clips played
// once hold their end pose (see end).
func (a *Animations) frame(i int, t float32, once bool) float32 {
	last := float32(max(a.Clips[i].KeyframeCount-1, 0))
	if last == 0 {
		return 0
	}
	f := t * a.frameRate()
	if once {
		return min(f, a.end(i))
	}
	return float32(math.Mod(float64(f), float64(last)))
}

// end is the keyframe clip i holds once it's played through: its last, or
// the one before when the last is a copy of the first. raylib samples a glTF
// clip's final keyframe past its last key, which gives the first pose again:
// right for a loop, but a clip that ends somewhere else would snap back to
// where it started.
func (a *Animations) end(i int) float32 {
	if len(a.ends) != len(a.Clips) {
		a.ends = make([]float32, len(a.Clips))
		for j := range a.Clips {
			c := &a.Clips[j]
			last := int(c.KeyframeCount) - 1
			a.ends[j] = float32(max(last, 0))
			if last < 2 || c.KeyframePoses == nil || c.BoneCount == 0 {
				continue
			}
			wrapped := true
			for bone := 0; bone < int(c.BoneCount) && wrapped; bone++ {
				wrapped = c.GetFramePose(last, bone) == c.GetFramePose(0, bone)
			}
			if wrapped {
				a.ends[j] = float32(last - 1)
			}
		}
	}
	return a.ends[i]
}

func loadAnimations(path string) (Animations, error) {
	clips := rl.LoadModelAnimations(path)
	if len(clips) == 0 {
		return Animations{}, errors.New("raylib found no animations in the file")
	}
	return Animations{Clips: clips, FrameRate: 60}, nil
}

func unloadAnimations(a *Animations) {
	if len(a.Clips) > 0 {
		rl.UnloadModelAnimations(a.Clips)
	}
}

// AnimationPlayer plays clips from Animations on the entity's Model3d.
//
//	player := render.AnimationPlayer{Animations: anims}
//	player.Play("Idle")
//	cmd.Spawn(illusion.C(render.Model3d{Model: model}), illusion.C(player), ...)
//
// Later, from a system: player.PlayOnce("Attack").FadeIn(0.1). Entities can
// share a Model and play different clips: each one's pose is applied just
// before it's drawn.
type AnimationPlayer struct {
	Animations asset.Handle[Animations]
	// Speed scales playback; 0 means 1, and negative plays backwards.
	Speed float32
	// Paused holds the current pose.
	Paused bool
	// ManualTime lets Seek drive the current pose while crossfades still advance.
	// Use for animation synchronized to physics displacement or a fixed-step clock.
	ManualTime bool

	// Pose optionally overrides the sampled skeleton in model space. Supply one
	// transform per bone after Animate, before attachments and drawing. Clothing
	// can share this read-only slice; nil retains ordinary clip playback.
	Pose []rl.Transform

	current, previous playback
	fade, fadeLength  float32 // crossfade progress and length, in seconds
	finished          bool
	fresh             bool // the last Play or PlayOnce started a clip, so FadeIn applies
}

type playback struct {
	clip string
	time float32
	once bool
}

// Play loops clip. If it's already looping, it carries on.
func (p *AnimationPlayer) Play(clip string) *AnimationPlayer {
	if p.current.clip == clip && !p.current.once {
		p.fresh = false
		return p
	}
	p.start(clip, false)
	return p
}

// PlayOnce plays clip once and holds its last frame, sending
// [AnimationFinished] when it ends. If it's already the current clip,
// it carries on; use Replay to start it over.
func (p *AnimationPlayer) PlayOnce(clip string) *AnimationPlayer {
	if p.current.clip == clip && p.current.once {
		p.fresh = false
		return p
	}
	p.start(clip, true)
	return p
}

// FadeIn blends from the previous clip to the one just started over seconds,
// instead of switching at once. Call it right after Play or PlayOnce; it does
// nothing if they didn't start a clip.
func (p *AnimationPlayer) FadeIn(seconds float32) *AnimationPlayer {
	if p.fresh && p.previous.clip != "" && seconds > 0 {
		p.fade, p.fadeLength = 0, seconds
	}
	return p
}

// Settle ends any crossfade at once, leaving the current clip alone to pose
// the model.
func (p *AnimationPlayer) Settle() {
	p.fade, p.fadeLength = 0, 0
}

// Replay starts the current clip over.
func (p *AnimationPlayer) Replay() {
	p.current.time = 0
	p.finished = false
}

// Seek jumps to seconds into the current clip.
func (p *AnimationPlayer) Seek(seconds float32) {
	p.current.time = max(seconds, 0)
	p.finished = false
}

// Clip returns the name of the clip playing, or "".
func (p *AnimationPlayer) Clip() string { return p.current.clip }

// Time returns how many seconds into the current clip playback is.
func (p *AnimationPlayer) Time() float32 { return p.current.time }

// Finished reports whether a PlayOnce clip has reached its end.
func (p *AnimationPlayer) Finished() bool { return p.finished }

func (p *AnimationPlayer) start(clip string, once bool) {
	switch {
	case p.fadeLength > 0 && p.fade < p.fadeLength/2:
		// Interrupting a crossfade that's still mostly the previous clip:
		// keep fading from that clip rather than jumping to the current one.
		// (raylib blends two clips at most, so the pose can't be carried over
		// exactly.)
	case p.current.clip != "":
		p.previous = p.current
	}
	p.current = playback{clip: clip, once: once}
	p.fade, p.fadeLength = 0, 0 // switch at once unless FadeIn follows
	p.finished = false
	p.fresh = true
}

// AnimationFinished is sent when a PlayOnce clip reaches its end.
type AnimationFinished struct {
	Entity ecs.Entity
	Clip   string
}

// Animate is the PostUpdate set that advances animation players and moves
// bone attachments. It runs before transform propagation.
const Animate illusion.SystemSet = "render.Animate"

// AdvanceAnimations and AttachBones split Animate so pose modifiers can run
// between clip sampling and attachments. Put modifiers in Animate as well.
const AdvanceAnimations illusion.SystemSet = "render.AdvanceAnimations"
const AttachBones illusion.SystemSet = "render.AttachBones"

func buildAnimation(app *illusion.App) {
	asset.RegisterLoader(app, loadAnimations, unloadAnimations)
	illusion.AddEvent[AnimationFinished](app)
	app.ConfigureSets(illusion.PostUpdate, Animate.Before(transform.Propagate))
	app.AddSystems(illusion.PostUpdate, illusion.Chain(
		illusion.Fn4(advanceAnimations).Named("render.advanceAnimations").InSet(AdvanceAnimations),
		illusion.Fn6(attachToBones).Named("render.attachToBones").InSet(AttachBones),
	).InSet(Animate))
}

// advanceAnimations moves every player's clock. The pose itself is applied
// when the entity is drawn (see poseModel).
func advanceAnimations(
	q *illusion.Query1[AnimationPlayer],
	store *illusion.Res[asset.Assets[Animations]],
	t *illusion.Res[illusion.Time],
	finished *illusion.EventWriter[AnimationFinished],
) {
	dt := t.Get().DeltaSecs()
	anims := store.Get()
	q.Each(func(e ecs.Entity, p *AnimationPlayer) {
		a := anims.Get(p.Animations)
		if a == nil || p.Paused || p.current.clip == "" {
			return
		}
		step := dt
		if p.Speed != 0 {
			step *= p.Speed
		}
		if i, ok := a.Clip(p.current.clip); ok {
			if !p.ManualTime {
				p.current.time = advance(p.current, step, a.Duration(i))
			}
			if p.current.once && !p.finished && p.current.time >= a.Duration(i) {
				p.finished = true
				finished.Send(AnimationFinished{Entity: e, Clip: p.current.clip})
			}
		}
		if p.fadeLength > 0 {
			if i, ok := a.Clip(p.previous.clip); ok {
				p.previous.time = advance(p.previous, step, a.Duration(i))
			}
			p.fade += dt // blending follows wall time, independently of stride speed
			if p.fade >= p.fadeLength {
				p.fade, p.fadeLength = 0, 0
			}
		}
	})
}

// advance moves playback b by step seconds (negative plays backwards) through
// a clip lasting duration, keeping the clock within the clip: once clips stop
// at either end, looping ones wrap.
func advance(b playback, step, duration float32) float32 {
	t := b.time + step
	switch {
	case b.once:
		return max(0, min(t, duration))
	case duration > 0:
		t = float32(math.Mod(float64(t), float64(duration)))
		if t < 0 {
			t += duration
		}
		return t
	}
	return 0
}

// fits reports whether clip i can pose model: it's rl.IsModelAnimationValid
// without the C call, plus the keyframe check raylib's update makes.
func (a *Animations) fits(model *rl.Model, i int) bool {
	return a.Clips[i].BoneCount == model.Skeleton.BoneCount && a.Clips[i].KeyframeCount > 0
}

// appliedPose is what poseModel last applied to a model, so it can skip
// re-skinning when nothing changed.
type appliedPose struct {
	custom                  bool
	scratch                 *poseScratch
	anims                   *Animations
	cur, prev               int
	frame, prevFrame, blend float32
}

// poseModel applies the player's pose to model, skinning its meshes. It does
// nothing if the clip isn't loaded or doesn't fit the model's skeleton, or if
// last (keyed by the model's meshes) shows the model already holds this pose.
func poseModel(model rl.Model, p *AnimationPlayer, a *Animations, last map[*rl.Mesh]appliedPose) {
	if len(p.Pose) > 0 && len(p.Pose) == int(model.Skeleton.BoneCount) {
		cached := last[model.Meshes]
		if cached.scratch == nil {
			cached.scratch = &poseScratch{}
		}
		applyCustomPose(model, p.Pose, cached.scratch)
		// Mark this as custom, so a following ordinary/shared-model draw
		// cannot reuse its old animation. Keep browser skinning buffers.
		last[model.Meshes] = appliedPose{custom: true, scratch: cached.scratch}
		return
	}
	want, ok := clipPose(model, p, a)
	if !ok {
		return
	}
	want.scratch = last[model.Meshes].scratch
	if got, ok := last[model.Meshes]; ok && got == want {
		return
	}
	last[model.Meshes] = want
	restore := routeSkinnedNormals(model)
	defer restore()
	if want.prev >= 0 {
		rl.UpdateModelAnimationEx(model, a.Clips[want.prev], want.prevFrame, a.Clips[want.cur], want.frame, want.blend)
		return
	}
	rl.UpdateModelAnimation(model, a.Clips[want.cur], want.frame)
}

// clipPose is the pose the player's clips put model in. ok is false if the
// clip isn't loaded or doesn't fit the model's skeleton.
func clipPose(model rl.Model, p *AnimationPlayer, a *Animations) (want appliedPose, ok bool) {
	cur, ok := a.Clip(p.current.clip)
	if !ok || !a.fits(&model, cur) {
		return appliedPose{}, false
	}
	want = appliedPose{anims: a, cur: cur, prev: -1, frame: a.frame(cur, p.current.time, p.current.once)}
	if p.fadeLength > 0 {
		if prev, ok := a.Clip(p.previous.clip); ok && a.fits(&model, prev) {
			want.prev, want.prevFrame = prev, a.frame(prev, p.previous.time, p.previous.once)
			want.blend = min(p.fade/p.fadeLength, 1)
		}
	}
	return want, true
}

// posedDraw is a model to draw, where, and the pose its clips put it in, if
// they do.
type posedDraw struct {
	entity ecs.Entity
	model  *Model
	matrix rl.Matrix
	pose   appliedPose
	clip   bool // posed by its clips: pose is set
}

// poseDraw is entity's model to draw at matrix, with the pose its player's
// clips put it in.
func poseDraw(entity ecs.Entity, model *Model, matrix rl.Matrix, players *illusion.Query1[AnimationPlayer], anims *asset.Assets[Animations]) posedDraw {
	d := posedDraw{entity: entity, model: model, matrix: matrix}
	if p, ok := players.Get(entity); ok && len(p.Pose) == 0 {
		if a := anims.Get(p.Animations); a != nil {
			d.pose, d.clip = clipPose(model.Model, p, a)
		}
	}
	return d
}

// byPose orders draws so the models their clips pose the same way are drawn
// together. A pose is skinned into the model's meshes, which every entity
// drawing the model shares, so it's skinned once for a run of them; drawn
// in the order they're found, a crowd playing a few clips would be skinned
// again for nearly every one. Draws not posed by clips keep their order,
// first.
func byPose(draws []posedDraw) {
	slices.SortStableFunc(draws, func(x, y posedDraw) int {
		a, b := x.pose, y.pose
		switch {
		case x.clip != y.clip:
			if x.clip {
				return 1
			}
			return -1
		case !x.clip:
			return 0
		}
		return cmp.Or(
			cmp.Compare(uintptr(unsafe.Pointer(x.model.Meshes)), uintptr(unsafe.Pointer(y.model.Meshes))),
			cmp.Compare(uintptr(unsafe.Pointer(a.anims)), uintptr(unsafe.Pointer(b.anims))),
			cmp.Compare(a.cur, b.cur),
			cmp.Compare(a.frame, b.frame),
			cmp.Compare(a.prev, b.prev),
			cmp.Compare(a.prevFrame, b.prevFrame),
			cmp.Compare(a.blend, b.blend),
		)
	})
}
