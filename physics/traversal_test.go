package physics

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/transform"
	"testing"
)

func TestTraversalQueriesResizeAndControlledMovement(t *testing.T) {
	app := newApp(t, func(cmd *illusion.Commands) {
		cmd.Spawn(illusion.C(tag("hero")), illusion.C(CharacterController{Radius: .3, Height: .9}), illusion.C(transform.FromXYZ(0, .47, 0)))
		cmd.Spawn(illusion.C(Static), illusion.C(Cuboid(3, .2, 3)), illusion.C(transform.FromXYZ(0, 1.2, 0)))
		cmd.Spawn(illusion.C(Static), illusion.C(Cuboid(.2, 3, 3)), illusion.C(transform.FromXYZ(3, 1.5, 0)))
	})
	run(app, .2)
	var checked bool
	app.AddSystems(illusion.FixedUpdate, illusion.Fn3(func(p *Physics, q *illusion.Query2[CharacterController, transform.Transform], cmd *illusion.Commands) {
		if checked {
			return
		}
		checked = true
		q.Each(func(e ecs.Entity, cc *CharacterController, tr *transform.Transform) {
			feet := tr.Translation.Y - cc.Height/2
			if p.ResizeCharacter(e, cc, tr, 1.8) {
				t.Error("standing inside ceiling should fail")
			}
			if cc.Height != .9 || tr.Translation.Y-cc.Height/2 != feet {
				t.Error("failed resize changed feet")
			}
			if !p.OverlapCapsuleExcluding(rl.Vector3{Y: .92}, .3, 1.8, e) {
				t.Error("standing capsule did not find ceiling")
			}
			if _, ok := p.SweepCapsuleExcluding(rl.Vector3{X: 1.8, Y: .47}, rl.Vector3{X: 2}, .3, .9, e); !ok {
				t.Error("capsule crossed wall")
			}
			if _, ok := p.SweepCapsuleExcluding(tr.Translation, rl.Vector3{Z: .2}, .3, .9, e); ok {
				t.Error("floor contact blocked horizontal sweep")
			}
			tr.Translation = rl.Vector3{X: -5, Y: .47}
			feet = tr.Translation.Y - cc.Height/2
			if !p.ResizeCharacter(e, cc, tr, 1.8) {
				t.Error("standing in clear space failed")
			}
			if abs32(tr.Translation.Y-cc.Height/2-feet) > .001 {
				t.Error("resize moved feet")
			}
			cc.Controlled = true
			cc.Walk = rl.Vector3{Y: 1.5}
		})
	}))
	run(app, .5)
	hero := find(app, "hero")
	cc := ecs.NewMap[CharacterController](app.World).Get(hero)
	y := translation(app, hero).Y
	if y < 1.5 || y > 1.85 {
		t.Fatalf("controlled climb y=%v", y)
	}
	cc.Walk = rl.Vector3{}
	run(app, .3)
	if abs32(translation(app, hero).Y-y) > .02 {
		t.Error("controlled hold drifted under gravity")
	}
	cc.Controlled = false
	run(app, .2)
	if translation(app, hero).Y >= y-.05 {
		t.Error("normal gravity did not resume")
	}
}
func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
