package render

import (
	"sort"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/transform"
)

// MaxPointLights is how many point lights light the scene at once: those
// that matter most to the camera.
const MaxPointLights = 8

// lightFade is how far, in metres, a light fades out over as another
// overtakes it for the last place, so lights don't pop on and off.
const lightFade = 4

// pointLight is a point light as the shaders get it.
type pointLight struct {
	position rl.Vector3
	color    rl.Vector3 // times its intensity and fade
	reach    float32
}

// selectLights picks the n lights that matter most seen from eye: those
// whose reach comes nearest it. Each is faded by how near the first light
// left out is to overtaking it, so they swap places smoothly.
func selectLights(eye rl.Vector3, lights []pointLight, n int) []pointLight {
	type scored struct {
		pointLight
		score float32
	}
	all := make([]scored, 0, len(lights))
	for _, l := range lights {
		if l.reach <= 0 {
			continue
		}
		all = append(all, scored{l, rl.Vector3Distance(eye, l.position) - l.reach})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].score < all[j].score })
	crowded := len(all) > n
	var cut float32
	if crowded {
		cut = all[n].score
		all = all[:n]
	}
	out := make([]pointLight, len(all))
	for i, l := range all {
		out[i] = l.pointLight
		if crowded {
			out[i].color = rl.Vector3Scale(l.color, min(1, (cut-l.score)/lightFade))
		}
	}
	return out
}

// gatherLights collects this frame's point lights, and picks those the
// camera at eye sees by.
func (r *renderer) gatherLights(eye rl.Vector3, q *illusion.Query2[PointLight, transform.GlobalTransform]) {
	r.allLights = r.allLights[:0]
	q.Each(func(_ ecs.Entity, l *PointLight, g *transform.GlobalTransform) {
		k := l.Intensity
		if k == 0 {
			k = 1
		}
		r.allLights = append(r.allLights, pointLight{position: g.Translation(), color: rgb(l.Color, k), reach: l.Range})
	})
	r.lights = selectLights(eye, r.allLights, MaxPointLights)
}

// sendLights sends a program the point lights: pointCount, and pointPos
// (xyz, and the reach in w) and pointColor for each.
func (r *renderer) sendLights(p *program) {
	count := p.loc("pointCount")
	if count < 0 {
		return
	}
	var pos [MaxPointLights * 4]float32
	var col [MaxPointLights * 3]float32
	for i, l := range r.lights {
		pos[4*i], pos[4*i+1], pos[4*i+2], pos[4*i+3] = l.position.X, l.position.Y, l.position.Z, l.reach
		col[3*i], col[3*i+1], col[3*i+2] = l.color.X, l.color.Y, l.color.Z
	}
	rl.SetShaderValue(p.shader, count, []float32{float32(len(r.lights))}, rl.ShaderUniformFloat)
	if loc := p.loc("pointPos"); loc >= 0 {
		rl.SetShaderValueV(p.shader, loc, pos[:], rl.ShaderUniformVec4, MaxPointLights)
	}
	if loc := p.loc("pointColor"); loc >= 0 {
		rl.SetShaderValueV(p.shader, loc, col[:], rl.ShaderUniformVec3, MaxPointLights)
	}
}
