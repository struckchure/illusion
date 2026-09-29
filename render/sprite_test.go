package render

import (
	"math"
	"testing"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/transform"
)

func TestGridFrames(t *testing.T) {
	f := GridFrames(3, 2, 16, 8)
	if len(f) != 6 || f[4] != (rl.Rectangle{X: 16, Y: 8, Width: 16, Height: 8}) {
		t.Fatalf("frames %v", f)
	}
}

func TestSpriteAnimation(t *testing.T) {
	frames := GridFrames(4, 1, 10, 10)
	app := illusion.New().AddSystems(illusion.PostUpdate, illusion.Fn2(animateSprites))
	app.AddSystems(illusion.Startup, illusion.Fn1(func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(Sprite{}), illusion.C(SpriteAnimation{Frames: frames, FPS: 10}))
		cmd.Spawn(illusion.C(Sprite{}), illusion.C(SpriteAnimation{Frames: frames, FPS: 10, Once: true}))
	}))
	app.Tick(0)
	for range 5 {
		app.Tick(100 * time.Millisecond) // one frame per tick
	}

	var loops, once *SpriteAnimation
	q := ecs.NewFilter2[SpriteAnimation, Sprite](app.World).Query()
	for q.Next() {
		a, s := q.Get()
		if s.Source != a.Frames[a.Frame()] {
			t.Fatalf("source %v doesn't match frame %d", s.Source, a.Frame())
		}
		if a.Once {
			once = a
		} else {
			loops = a
		}
	}
	// Five steps: looping wraps 0→1→2→3→0→1; once stops at 3.
	if loops.Frame() != 1 || once.Frame() != 3 || !once.Finished() {
		t.Fatalf("looping at %d, once at %d", loops.Frame(), once.Frame())
	}
}

func TestPlanar(t *testing.T) {
	tr := transform.FromXY(10, 20)
	tr.Scale = rl.Vector3{X: 2, Y: 3, Z: 1}
	tr.RotateZ(math.Pi / 6)
	g := transform.GlobalTransform{Matrix: tr.Matrix()}
	pos, angle, scale := planar(&g)
	if pos != (rl.Vector2{X: 10, Y: 20}) {
		t.Fatalf("pos %v", pos)
	}
	if math.Abs(float64(angle)-math.Pi/6) > 1e-5 {
		t.Fatalf("angle %v", angle)
	}
	if math.Abs(float64(scale.X-2)) > 1e-5 || math.Abs(float64(scale.Y-3)) > 1e-5 {
		t.Fatalf("scale %v", scale)
	}
}
