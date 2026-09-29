// A spinning cube driven entirely by components.
//
// Left/Right change the spin speed, Space changes the color, Escape quits.
package main

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/engine"
	"github.com/struckchure/illusion/input"
	"github.com/struckchure/illusion/render"
	"github.com/struckchure/illusion/transform"
	"github.com/struckchure/illusion/window"
)

// Spin rotates an entity around the Y axis.
type Spin struct {
	Speed float32 // radians per second
}

var palette = []rl.Color{rl.Orange, rl.SkyBlue, rl.Lime, rl.Pink, rl.Gold}

func main() {
	illusion.New().
		AddPlugins(engine.DefaultPlugins(engine.Config{Window: window.Config{Title: "Illusion: cube", MSAA: true}})).
		AddSystems(illusion.Startup, illusion.Fn3(setup)).
		AddSystems(illusion.Update, illusion.Fn2(spin), illusion.Fn3(controls)).
		AddSystems(illusion.Render,
			illusion.Fn0(drawGrid).InSet(render.Draw3D),
			illusion.Fn1(drawHUD).InSet(render.Draw2D),
		).
		Run()
}

func setup(
	cmd *illusion.Commands,
	meshes *illusion.Res[asset.Assets[render.Mesh]],
	materials *illusion.Res[asset.Assets[render.StandardMaterial]],
) {
	cmd.Spawn(
		illusion.C(render.Camera3d{}),
		illusion.C(transform.FromXYZ(4, 3, 6).LookingAt(rl.Vector3{}, transform.Up)),
	)
	cmd.Spawn(
		illusion.C(render.DirectionalLight{Color: rl.White}),
		illusion.C(transform.FromXYZ(0, 0, 0).LookingAt(rl.Vector3{X: -1, Y: -3, Z: -2}, transform.Up)),
	)
	cmd.Spawn(
		illusion.C(render.Mesh3d{Mesh: meshes.Get().Add(render.Cuboid(2, 2, 2))}),
		illusion.C(render.MeshMaterial3d{Material: materials.Get().Add(render.StandardMaterial{BaseColor: palette[0]})}),
		illusion.C(transform.FromXYZ(0, 1.25, 0)),
		illusion.C(Spin{Speed: 1}),
	)
	cmd.Spawn(
		illusion.C(render.Mesh3d{Mesh: meshes.Get().Add(render.Plane(10, 10))}),
		illusion.C(render.MeshMaterial3d{Material: materials.Get().Add(render.StandardMaterial{BaseColor: rl.DarkGray})}),
		illusion.C(transform.FromXYZ(0, -0.01, 0)), // just under the grid lines
	)
}

func spin(q *illusion.Query2[Spin, transform.Transform], t *illusion.Res[illusion.Time]) {
	dt := t.Get().DeltaSecs()
	query := q.Iter()
	for query.Next() {
		s, tr := query.Get()
		tr.RotateY(s.Speed * dt)
	}
}

func controls(
	keys *illusion.Res[input.Keys],
	q *illusion.Query2[Spin, render.MeshMaterial3d],
	materials *illusion.Res[asset.Assets[render.StandardMaterial]],
) {
	k := keys.Get()
	query := q.Iter()
	for query.Next() {
		s, m := query.Get()
		if k.JustPressed(rl.KeyRight) {
			s.Speed += 0.5
		}
		if k.JustPressed(rl.KeyLeft) {
			s.Speed -= 0.5
		}
		if k.JustPressed(rl.KeySpace) {
			mat := materials.Get().Get(m.Material)
			for i, c := range palette {
				if c == mat.BaseColor {
					mat.BaseColor = palette[(i+1)%len(palette)]
					break
				}
			}
		}
	}
}

func drawGrid() {
	rl.DrawGrid(10, 1)
}

func drawHUD(q *illusion.Query1[Spin]) {
	rl.DrawFPS(10, 10)
	if _, s, ok := q.Single(); ok {
		rl.DrawText(fmt.Sprintf("spin %.1f rad/s   left/right: speed   space: color", s.Speed), 10, 36, 20, rl.RayWhite)
	}
}
