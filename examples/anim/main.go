// A row of tentacles playing skeletal animations. They share one model; each
// has its own AnimationPlayer, so each holds its own pose. A glowing orb rides
// each tip on a BoneAttachment.
//
// 1 sways them, 2 curls them (both crossfade), Space makes a random one
// strike, S strikes them all in a ripple. Escape quits.
//
// The model is generated: go run examples/anim/tentacle.go.
package main

import (
	"fmt"
	"math/rand/v2"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/defaults"
	"github.com/struckchure/illusion/input"
	"github.com/struckchure/illusion/render"
	"github.com/struckchure/illusion/transform"
	"github.com/struckchure/illusion/window"
)

const count = 7

// Tentacle remembers which looping clip to return to after a strike.
type Tentacle struct {
	Loop  string
	Delay float32 // seconds until a queued strike
}

func main() {
	illusion.New().
		AddPlugins(defaults.Plugins(defaults.Config{
			Window:    window.Config{Title: "Illusion: animation", MSAA: true},
			AssetRoot: "examples/assets",
		})).
		AddSystems(illusion.Startup, illusion.Fn5(setup)).
		AddSystems(illusion.Update, illusion.Fn2(controls), illusion.Fn2(strikeQueued), illusion.Fn2(backToLoop)).
		AddSystems(illusion.Render,
			illusion.Fn0(drawGrid).InSet(render.Draw3D),
			illusion.Fn1(drawHUD).InSet(render.Draw2D),
		).
		Run()
}

func setup(
	cmd *illusion.Commands,
	models *asset.Loader[render.Model],
	anims *asset.Loader[render.Animations],
	meshes *illusion.Res[asset.Assets[render.Mesh]],
	materials *illusion.Res[asset.Assets[render.StandardMaterial]],
) {
	cmd.Spawn(
		illusion.C(render.Camera3d{}),
		illusion.C(transform.FromXYZ(0, 3.2, 7.5).LookingAt(rl.Vector3{Y: 1.2}, transform.Up)),
	)
	cmd.Spawn(
		illusion.C(render.DirectionalLight{Color: rl.White}),
		illusion.C(transform.Identity().LookingAt(rl.Vector3{X: -1, Y: -2, Z: -2}, transform.Up)),
	)
	cmd.Spawn(
		illusion.C(render.Mesh3d{Mesh: meshes.Get().Add(render.Plane(12, 6))}),
		illusion.C(render.MeshMaterial3d{Material: materials.Get().Add(render.StandardMaterial{BaseColor: rl.NewColor(40, 70, 60, 255)})}),
		illusion.C(transform.Identity()),
	)

	model := models.MustLoad("tentacle.glb")
	clips := anims.MustLoad("tentacle.glb")
	orb := meshes.Get().Add(render.Sphere(0.1))
	glow := materials.Get().Add(render.StandardMaterial{BaseColor: rl.NewColor(255, 220, 120, 255), Unlit: true})
	for i := range count {
		p := render.AnimationPlayer{Animations: clips, Speed: 0.85 + 0.3*rand.Float32()}
		p.Play("Sway")
		p.Seek(rand.Float32() * 2) // out of step with each other
		x := float32(i) - float32(count-1)/2
		cmd.Spawn(
			illusion.C(render.Model3d{Model: model}),
			illusion.C(p),
			illusion.C(Tentacle{Loop: "Sway"}),
			illusion.C(transform.FromXYZ(x*1.1, 0, -float32(i%2)*0.8)),
		).WithChild(
			illusion.C(render.Mesh3d{Mesh: orb}),
			illusion.C(render.MeshMaterial3d{Material: glow}),
			illusion.C(transform.Identity()),
			// The top bone starts 2.5 up; the tip is 0.58 above it.
			illusion.C(render.BoneAttachment{Bone: "Bone5", Offset: transform.FromXYZ(0, 0.66, 0)}),
		)
	}
}

func controls(keys *illusion.Res[input.Keys], q *illusion.Query2[render.AnimationPlayer, Tentacle]) {
	k := keys.Get()
	loop := ""
	switch {
	case k.JustPressed(rl.KeyOne):
		loop = "Sway"
	case k.JustPressed(rl.KeyTwo):
		loop = "Curl"
	}
	var all []*render.AnimationPlayer
	var tentacles []*Tentacle
	q.Each(func(_ ecs.Entity, p *render.AnimationPlayer, t *Tentacle) {
		all, tentacles = append(all, p), append(tentacles, t)
		if loop != "" {
			t.Loop = loop
			if !isStriking(p) {
				p.Play(loop).FadeIn(0.4)
			}
		}
	})
	if len(all) == 0 {
		return
	}
	if k.JustPressed(rl.KeySpace) {
		strike(all[rand.IntN(len(all))])
	}
	if k.JustPressed(rl.KeyS) {
		for i, t := range tentacles {
			t.Delay = 0.001 + float32(i)*0.08
		}
	}
}

func isStriking(p *render.AnimationPlayer) bool { return p.Clip() == "Strike" && !p.Finished() }

func strike(p *render.AnimationPlayer) {
	if p.Clip() == "Strike" {
		p.Replay()
		return
	}
	p.PlayOnce("Strike").FadeIn(0.08)
}

// strikeQueued fires the ripple started with S.
func strikeQueued(q *illusion.Query2[render.AnimationPlayer, Tentacle], t *illusion.Res[illusion.Time]) {
	dt := t.Get().DeltaSecs()
	q.Each(func(_ ecs.Entity, p *render.AnimationPlayer, tn *Tentacle) {
		if tn.Delay <= 0 {
			return
		}
		if tn.Delay -= dt; tn.Delay <= 0 {
			strike(p)
		}
	})
}

// backToLoop returns a tentacle to its looping clip once a strike ends.
func backToLoop(done *illusion.EventReader[render.AnimationFinished], q *illusion.Query2[render.AnimationPlayer, Tentacle]) {
	for e := range done.Read() {
		if p, t, ok := q.Get(e.Entity); ok && e.Clip == "Strike" {
			p.Play(t.Loop).FadeIn(0.35)
		}
	}
}

func drawGrid() { rl.DrawGrid(20, 0.5) }

func drawHUD(q *illusion.Query1[render.AnimationPlayer]) {
	striking := 0
	clip := ""
	q.Each(func(_ ecs.Entity, p *render.AnimationPlayer) {
		if isStriking(p) {
			striking++
		} else {
			clip = p.Clip()
		}
	})
	rl.DrawFPS(10, 10)
	rl.DrawText(fmt.Sprintf("%d tentacles, one model   playing %s   striking %d", count, clip, striking), 10, 36, 20, rl.RayWhite)
	rl.DrawText("1 sway   2 curl   space strike   s ripple", 10, 62, 20, rl.Gray)
}
