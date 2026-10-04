package render

import (
	"math"
	"testing"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
)

func clip(name string, keyframes int32) rl.ModelAnimation {
	var a rl.ModelAnimation
	copy(a.Name[:], name)
	a.KeyframeCount = keyframes
	return a
}

// newAnimApp returns an app running the animation systems (no window needed:
// poses are only applied when drawing) with one entity playing from clips
// "Walk" (1s) and "Jump" (0.5s), and the AnimationFinished events it sent.
func newAnimApp(t *testing.T, setup func(p *AnimationPlayer)) (*illusion.App, ecs.Entity, *[]AnimationFinished) {
	t.Helper()
	app := illusion.New()
	buildAnimation(app)
	var finished []AnimationFinished
	app.AddSystems(illusion.Last, illusion.Fn1(func(r *illusion.EventReader[AnimationFinished]) {
		for e := range r.Read() {
			finished = append(finished, e)
		}
	}))
	app.AddSystems(illusion.Startup, illusion.Fn2(func(cmd *illusion.Commands, store *illusion.Res[asset.Assets[Animations]]) {
		h := store.Get().Add(Animations{Clips: []rl.ModelAnimation{clip("Walk", 61), clip("Jump", 31)}, FrameRate: 60})
		p := AnimationPlayer{Animations: h}
		setup(&p)
		cmd.Spawn(illusion.C(p))
	}))
	app.Tick(0) // startup
	var e ecs.Entity
	query := ecs.NewFilter1[AnimationPlayer](app.World).Query()
	for query.Next() {
		e = query.Entity()
	}
	return app, e, &finished
}

func player(app *illusion.App, e ecs.Entity) *AnimationPlayer {
	return ecs.NewMap[AnimationPlayer](app.World).Get(e)
}

func run(app *illusion.App, seconds float64) {
	for range int(math.Round(seconds * 60)) {
		app.Tick(time.Second / 60)
	}
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }

func TestAnimationLoops(t *testing.T) {
	app, e, _ := newAnimApp(t, func(p *AnimationPlayer) { p.Play("Walk") })
	run(app, 1.25)
	p := player(app, e)
	if !near(p.Time(), 0.25) || p.Finished() {
		t.Fatalf("after 1.25s of a 1s loop: time %v finished %v", p.Time(), p.Finished())
	}
	a := Animations{Clips: []rl.ModelAnimation{clip("Walk", 61)}}
	if f := a.frame(0, p.Time(), false); !near(f, 15) {
		t.Fatalf("frame %v, want 15", f)
	}
	// Playing the clip that's already looping carries on rather than restarting.
	p.Play("Walk")
	if !near(p.Time(), 0.25) {
		t.Fatalf("Play restarted the running clip: time %v", p.Time())
	}
}

func TestAnimationOnceFinishesAndHolds(t *testing.T) {
	app, e, finished := newAnimApp(t, func(p *AnimationPlayer) { p.PlayOnce("Jump") })
	run(app, 1)
	p := player(app, e)
	if !p.Finished() || !near(p.Time(), 0.5) {
		t.Fatalf("time %v finished %v, want 0.5 and finished", p.Time(), p.Finished())
	}
	if len(*finished) != 1 || (*finished)[0].Entity != e || (*finished)[0].Clip != "Jump" {
		t.Fatalf("finished events %+v, want one for Jump", *finished)
	}
	a := Animations{Clips: []rl.ModelAnimation{clip("Walk", 61), clip("Jump", 31)}}
	if f := a.frame(1, p.Time(), true); f != 30 {
		t.Fatalf("held frame %v, want the last (30)", f)
	}

	p.Replay()
	run(app, 1)
	if len(*finished) != 2 {
		t.Fatalf("Replay didn't finish again: %d events", len(*finished))
	}
}

// raylib's glTF loader ends every clip on a copy of its first keyframe. A
// clip played once holds the pose before that copy, not its start.
func TestAnimationOnceHoldsEndPoseOfWrappedClip(t *testing.T) {
	poses := func(ys ...float32) (rl.ModelAnimation, []rl.ModelAnimPose) {
		frames := make([]rl.ModelAnimPose, len(ys))
		for i, y := range ys {
			frames[i] = &rl.Transform{Translation: rl.Vector3{Y: y}, Rotation: rl.QuaternionIdentity(), Scale: rl.Vector3One()}
		}
		c := clip("Climb", int32(len(ys)))
		c.BoneCount, c.KeyframePoses = 1, &frames[0]
		return c, frames
	}
	wrapped, keepA := poses(0, 1, 2, 3, 0)
	whole, keepB := poses(0, 1, 2, 3, 4)
	_, _ = keepA, keepB
	a := Animations{Clips: []rl.ModelAnimation{wrapped, whole}}
	for i, want := range []float32{3, 4} {
		f := a.frame(i, 10, true)
		if y := sample(a.Clips[i], f, 0, true).Translation.Y; y != want {
			t.Fatalf("clip %d held at frame %v, height %v; want its end pose, %v", i, f, y, want)
		}
	}
	// Looping, both still wrap at the last keyframe.
	if f := a.frame(0, 4.5/60, false); !near(f, 0.5) {
		t.Fatalf("looping frame %v, want 0.5", f)
	}
}

func TestAnimationFadeAndSpeed(t *testing.T) {
	app, e, _ := newAnimApp(t, func(p *AnimationPlayer) { p.Play("Walk") })
	run(app, 0.5)
	p := player(app, e)
	p.PlayOnce("Jump").FadeIn(0.25)
	if p.Clip() != "Jump" || p.previous.clip != "Walk" || p.fadeLength != 0.25 {
		t.Fatalf("fade not set up: %+v", p)
	}
	run(app, 0.1)
	if !near(p.previous.time, 0.6) || !near(p.fade, 0.1) {
		t.Fatalf("during the fade the old clip keeps playing: prev %v fade %v", p.previous.time, p.fade)
	}
	run(app, 0.2)
	if p.fadeLength != 0 {
		t.Fatalf("fade didn't end: %v/%v", p.fade, p.fadeLength)
	}

	p.Play("Walk")
	p.Speed = 2
	run(app, 0.25)
	if !near(p.Time(), 0.5) {
		t.Fatalf("at speed 2, 0.25s moved %v, want 0.5", p.Time())
	}
	p.Paused = true
	run(app, 0.25)
	if !near(p.Time(), 0.5) {
		t.Fatalf("paused player moved to %v", p.Time())
	}
}

func TestAnimationsClipLookup(t *testing.T) {
	a := Animations{Clips: []rl.ModelAnimation{clip("Walk", 61), clip("Jump", 31)}}
	if i, ok := a.Clip("Jump"); !ok || i != 1 {
		t.Fatalf("Clip(Jump) = %v, %v", i, ok)
	}
	if _, ok := a.Clip("Swim"); ok {
		t.Fatal("found a clip that doesn't exist")
	}
	if d := a.Duration(0); !near(d, 1) {
		t.Fatalf("Walk lasts %v, want 1s at the default 60 keyframes per second", d)
	}
	if names := a.Names(); len(names) != 2 || names[0] != "Walk" || names[1] != "Jump" {
		t.Fatalf("names %v", names)
	}
}

func TestAnimationFadeInNeedsANewClip(t *testing.T) {
	p := &AnimationPlayer{}
	p.Play("Walk")
	p.Play("Jump").FadeIn(0.2)
	p.fade, p.fadeLength = 0, 0 // the fade has ended
	p.Play("Jump").FadeIn(0.4)  // already playing: nothing to fade from
	if p.fadeLength != 0 {
		t.Fatalf("FadeIn after a Play that started nothing faded from %q", p.previous.clip)
	}
}

func TestAnimationPlaysBackwards(t *testing.T) {
	app, e, _ := newAnimApp(t, func(p *AnimationPlayer) { p.Play("Walk"); p.Speed = -1 })
	run(app, 0.25)
	if tm := player(app, e).Time(); !near(tm, 0.75) {
		t.Fatalf("a 1s loop played backwards for 0.25s is at %v, want 0.75", tm)
	}
	app2, e2, _ := newAnimApp(t, func(p *AnimationPlayer) { p.PlayOnce("Jump"); p.Speed = -1 })
	run(app2, 0.25)
	if tm := player(app2, e2).Time(); tm != 0 {
		t.Fatalf("a once clip played backwards should stop at 0, got %v", tm)
	}
}

func TestAnimationInterruptedFadeKeepsDominantClip(t *testing.T) {
	p := &AnimationPlayer{}
	p.Play("Walk")
	p.Play("Run").FadeIn(1)
	p.fade = 0.2 // still mostly Walk
	p.PlayOnce("Jump").FadeIn(0.2)
	if p.previous.clip != "Walk" {
		t.Fatalf("interrupting early in a fade should fade from Walk, got %q", p.previous.clip)
	}
}

func TestManualTimeHoldsPoseButCompletesBlend(t *testing.T) {
	app, e, _ := newAnimApp(t, func(p *AnimationPlayer) { p.Play("Walk") })
	p := player(app, e)
	p.PlayOnce("Jump").FadeIn(.2)
	p.ManualTime = true
	p.Seek(.12)
	run(app, .3)
	if !near(p.Time(), .12) || p.fadeLength != 0 {
		t.Fatalf("manual time=%v fade=%v", p.Time(), p.fadeLength)
	}
	p.Paused = true
	p.Play("Walk").FadeIn(.2)
	run(app, .1)
	if p.fade != 0 {
		t.Fatal("paused manual blend advanced")
	}
	p.Paused, p.ManualTime = false, false
	run(app, .1)
	if !near(p.Time(), .1) {
		t.Fatal("normal playback did not resume")
	}
}
