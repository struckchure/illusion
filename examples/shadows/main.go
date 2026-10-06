// Shadows and point lights: a low sun throws the blocks' shadows across the
// ground, a lamp on a post lights what's round it, and its glass glows.
//
// Left/Right turn the camera, Escape quits.
package main

import (
	"math"

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

// Orbit turns the camera round the middle.
type Orbit struct {
	Angle float32
}

func main() {
	illusion.New().
		AddPlugins(defaults.Plugins(defaults.Config{Window: window.Config{Title: "Illusion: shadows", MSAA: true}})).
		InsertResource(
			illusion.R(&render.Shadows{Size: 2048, Range: 15}),
			illusion.R(&render.AmbientLight{Color: rl.NewColor(150, 160, 200, 255), Brightness: 0.3}),
			illusion.R(&render.ClearColor{Color: rl.NewColor(120, 150, 190, 255)}),
		).
		AddSystems(illusion.Startup, illusion.Fn3(setup)).
		AddSystems(illusion.Update, illusion.Fn3(orbit)).
		Run()
}

func setup(
	cmd *illusion.Commands,
	meshes *illusion.Res[asset.Assets[render.Mesh]],
	materials *illusion.Res[asset.Assets[render.StandardMaterial]],
) {
	m, mat := meshes.Get(), materials.Get()
	paint := func(c rl.Color) render.MeshMaterial3d {
		return render.MeshMaterial3d{Material: mat.Add(render.StandardMaterial{BaseColor: c})}
	}
	cmd.Spawn(
		illusion.C(render.Camera3d{}),
		illusion.C(transform.FromXYZ(0, 6, 14).LookingAt(rl.Vector3{}, transform.Up)),
		illusion.C(Orbit{}),
	)
	// A low sun, so the shadows are long.
	cmd.Spawn(
		illusion.C(render.DirectionalLight{Color: rl.NewColor(255, 220, 180, 255), Brightness: 1.2}),
		illusion.C(transform.Identity().LookingAt(rl.Vector3{X: -1, Y: -0.5, Z: -0.4}, transform.Up)),
	)
	cmd.Spawn(
		illusion.C(render.Mesh3d{Mesh: m.Add(render.Plane(40, 40))}),
		illusion.C(paint(rl.NewColor(170, 160, 140, 255))),
		illusion.C(transform.Identity()),
	)
	for i, b := range []struct{ x, z, w, h float32 }{{-4, -2, 1.5, 3}, {0, 2, 2, 1.5}, {3, -3, 1, 4}, {-1, -5, 3, 2}} {
		cmd.Spawn(
			illusion.C(render.Mesh3d{Mesh: m.Add(render.Cuboid(b.w, b.h, b.w))}),
			illusion.C(paint([]rl.Color{rl.Orange, rl.SkyBlue, rl.Lime, rl.Pink}[i])),
			illusion.C(transform.FromXYZ(b.x, b.h/2, b.z)),
		)
	}
	// A lamp on a post: it lights what's round it, and its glass glows.
	cmd.Spawn(
		illusion.C(render.Mesh3d{Mesh: m.Add(render.Cylinder(0.08, 2.4))}),
		illusion.C(paint(rl.DarkGray)),
		illusion.C(transform.FromXYZ(4, 0, 3)),
	)
	cmd.Spawn(
		illusion.C(render.Mesh3d{Mesh: m.Add(render.Sphere(0.25))}),
		illusion.C(render.MeshMaterial3d{Material: mat.Add(render.StandardMaterial{BaseColor: rl.NewColor(255, 200, 120, 255), Emissive: rl.White})}),
		illusion.C(transform.FromXYZ(4, 2.6, 3)),
		illusion.C(render.NotShadowCaster{}),
		illusion.C(render.PointLight{Color: rl.NewColor(255, 180, 90, 255), Intensity: 1.5, Range: 6}),
	)
}

func orbit(q *illusion.Query2[Orbit, transform.Transform], keys *illusion.Res[input.Keys], t *illusion.Res[illusion.Time]) {
	k := keys.Get()
	q.Each(func(_ ecs.Entity, o *Orbit, tr *transform.Transform) {
		if k.Pressed(rl.KeyLeft) {
			o.Angle -= t.Get().DeltaSecs()
		}
		if k.Pressed(rl.KeyRight) {
			o.Angle += t.Get().DeltaSecs()
		}
		s, c := math.Sincos(float64(o.Angle))
		*tr = transform.FromXYZ(14*float32(s), 6, 14*float32(c)).LookingAt(rl.Vector3{}, transform.Up)
	})
}
