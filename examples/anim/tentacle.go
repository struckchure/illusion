//go:build ignore

// Generates examples/assets/tentacle.glb: a tapered tentacle with six bones
// and three clips (Sway and Curl loop; Strike is a one-shot whip). Run it from the
// repository root:
//
//	go run examples/anim/tentacle.go
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
)

const (
	bones   = 6
	boneLen = 0.5
	sides   = 8
	rings   = bones*2 + 1 // two rings per bone, plus the top
	radius  = 0.22
)

func main() {
	var pos, nrm, weights []float32
	var joints, colors []uint8
	var idx []uint16
	for r := range rings {
		y := float32(r) * boneLen / 2
		rad := radius * (1 - 0.8*float32(r)/float32(rings-1))
		b0, b1, w1 := skin(y)
		for s := range sides {
			a := 2 * math.Pi * float64(s) / sides
			x, z := float32(math.Cos(a)), float32(math.Sin(a))
			pos = append(pos, rad*x, y, rad*z)
			nrm = append(nrm, x, 0, z)
			joints = append(joints, b0, b1, 0, 0)
			weights = append(weights, 1-w1, w1, 0, 0)
			t := float32(r) / float32(rings-1)
			colors = append(colors, uint8(90+150*t), uint8(40+60*t), uint8(140+60*t), 255)
		}
	}
	for r := range rings - 1 {
		for s := range sides {
			a, b := uint16(r*sides+s), uint16(r*sides+(s+1)%sides)
			idx = append(idx, a, b+sides, b, a, a+sides, b+sides)
		}
	}
	// A cap on top.
	tip := uint16(len(pos) / 3)
	pos = append(pos, 0, float32(rings-1)*boneLen/2+0.08, 0)
	nrm = append(nrm, 0, 1, 0)
	joints = append(joints, bones-1, 0, 0, 0)
	weights = append(weights, 1, 0, 0, 0)
	colors = append(colors, 245, 110, 210, 255)
	for s := range sides {
		top := uint16((rings - 1) * sides)
		idx = append(idx, top+uint16(s), tip, top+uint16((s+1)%sides))
	}

	ibm := make([]float32, 0, 16*bones)
	for b := range bones {
		ibm = append(ibm, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, -float32(b)*boneLen, 0, 1)
	}

	g := &gltf{}
	n := len(pos) / 3
	posAcc := g.accessor(pos, 5126, n, "VEC3", false)
	g.Accessors[posAcc]["min"], g.Accessors[posAcc]["max"] = bounds(pos)
	attrs := map[string]int{
		"POSITION":  posAcc,
		"NORMAL":    g.accessor(nrm, 5126, n, "VEC3", false),
		"COLOR_0":   g.accessor(colors, 5121, n, "VEC4", true),
		"JOINTS_0":  g.accessor(joints, 5121, n, "VEC4", false),
		"WEIGHTS_0": g.accessor(weights, 5126, n, "VEC4", false),
	}
	indices := g.accessor(idx, 5123, len(idx), "SCALAR", false)
	ibmAcc := g.accessor(ibm, 5126, bones, "MAT4", false)

	nodes := []map[string]any{
		{"name": "Tentacle", "mesh": 0, "skin": 0},
		{"name": "Armature", "children": []int{2}},
	}
	var jointNodes []int
	for b := range bones {
		node := map[string]any{"name": "Bone" + string(rune('0'+b))}
		if b > 0 {
			node["translation"] = []float32{0, boneLen, 0}
		}
		if b < bones-1 {
			node["children"] = []int{3 + b}
		}
		nodes = append(nodes, node)
		jointNodes = append(jointNodes, 2+b)
	}

	anims := []map[string]any{
		g.clip("Sway", 2, 0.1, func(t float64, b int) quat {
			phase := 2*math.Pi*t/2 - float64(b)*0.6
			return mul(axisAngle(0, 0, 1, 12*math.Sin(phase)), axisAngle(1, 0, 0, 6*math.Cos(phase)))
		}),
		g.clip("Curl", 1.5, 0.05, func(t float64, b int) quat {
			return axisAngle(1, 0, 0, 22*(0.5-0.5*math.Cos(2*math.Pi*t/1.5)))
		}),
		g.clip("Strike", 1.0, 0.02, func(t float64, b int) quat {
			t -= float64(b) * 0.03 // the whip travels up the tentacle
			return axisAngle(1, 0, 0, strike(t))
		}),
	}
	for i := range anims {
		for _, ch := range anims[i]["channels"].([]map[string]any) {
			target := ch["target"].(map[string]any)
			target["node"] = jointNodes[target["node"].(int)]
		}
	}

	doc := map[string]any{
		"asset":  map[string]string{"version": "2.0", "generator": "illusion examples/anim/tentacle.go"},
		"scene":  0,
		"scenes": []map[string]any{{"nodes": []int{0, 1}}},
		"nodes":  nodes,
		"meshes": []map[string]any{{"primitives": []map[string]any{{"attributes": attrs, "indices": indices, "material": 0}}}},
		"materials": []map[string]any{{"pbrMetallicRoughness": map[string]any{
			"baseColorFactor": []float32{1, 1, 1, 1}, "metallicFactor": 0,
		}}},
		"skins":       []map[string]any{{"joints": jointNodes, "inverseBindMatrices": ibmAcc}},
		"animations":  anims,
		"accessors":   g.Accessors,
		"bufferViews": g.Views,
		"buffers":     []map[string]int{{"byteLength": g.bin.Len()}},
	}
	if err := os.WriteFile("examples/assets/tentacle.glb", glb(doc, g.bin.Bytes()), 0o644); err != nil {
		panic(err)
	}
}

// skin returns the two bones a ring at height y follows and the second's
// weight: rings near a joint blend the bones on either side of it.
func skin(y float32) (uint8, uint8, float32) {
	s := y / boneLen
	b := min(int(s), bones-1)
	f := s - float32(b)
	if b > 0 && f < 0.5 {
		return uint8(b - 1), uint8(b), 0.5 + f
	}
	return uint8(b), uint8(b), 0
}

// strike is each bone's bend over the Strike clip, in degrees. Six bones
// compound it, so the lash stays small enough to keep the tip above ground.
func strike(t float64) float64 {
	switch {
	case t <= 0:
		return 0
	case t < 0.2: // wind up
		return -12 * math.Sin(t/0.2*math.Pi/2)
	case t < 0.4: // lash forward
		return -12 + 34*math.Sin((t-0.2)/0.2*math.Pi/2)
	case t < 0.8: // settle
		return 22 * (0.5 + 0.5*math.Cos((t-0.4)/0.4*math.Pi))
	}
	return 0
}

type quat [4]float32

func axisAngle(x, y, z, degrees float64) quat {
	h := degrees * math.Pi / 360
	s := math.Sin(h)
	return quat{float32(x * s), float32(y * s), float32(z * s), float32(math.Cos(h))}
}

func mul(a, b quat) quat {
	return quat{
		a[3]*b[0] + a[0]*b[3] + a[1]*b[2] - a[2]*b[1],
		a[3]*b[1] - a[0]*b[2] + a[1]*b[3] + a[2]*b[0],
		a[3]*b[2] + a[0]*b[1] - a[1]*b[0] + a[2]*b[3],
		a[3]*b[3] - a[0]*b[0] - a[1]*b[1] - a[2]*b[2],
	}
}

type gltf struct {
	bin       bytes.Buffer
	Views     []map[string]int
	Accessors []map[string]any
}

// accessor appends data to the binary buffer and describes it.
func (g *gltf) accessor(data any, componentType, count int, typ string, normalized bool) int {
	for g.bin.Len()%4 != 0 {
		g.bin.WriteByte(0)
	}
	start := g.bin.Len()
	binary.Write(&g.bin, binary.LittleEndian, data)
	g.Views = append(g.Views, map[string]int{"buffer": 0, "byteOffset": start, "byteLength": g.bin.Len() - start})
	acc := map[string]any{"bufferView": len(g.Views) - 1, "componentType": componentType, "count": count, "type": typ}
	if normalized {
		acc["normalized"] = true
	}
	g.Accessors = append(g.Accessors, acc)
	return len(g.Accessors) - 1
}

// clip samples rotate(t, bone) every step seconds over length seconds into
// one rotation channel per bone. Channel targets are bone indices, mapped to
// nodes by the caller.
func (g *gltf) clip(name string, length, step float64, rotate func(t float64, bone int) quat) map[string]any {
	var times []float32
	for t := 0.0; t < length+step/2; t += step {
		times = append(times, float32(min(t, length)))
	}
	input := g.accessor(times, 5126, len(times), "SCALAR", false)
	g.Accessors[input]["min"], g.Accessors[input]["max"] = []float32{0}, []float32{float32(length)}
	var samplers, channels []map[string]any
	for b := range bones {
		var rot []quat
		for _, t := range times {
			rot = append(rot, rotate(float64(t), b))
		}
		output := g.accessor(rot, 5126, len(rot), "VEC4", false)
		samplers = append(samplers, map[string]any{"input": input, "output": output, "interpolation": "LINEAR"})
		channels = append(channels, map[string]any{"sampler": b, "target": map[string]any{"node": b, "path": "rotation"}})
	}
	return map[string]any{"name": name, "samplers": samplers, "channels": channels}
}

func bounds(v []float32) ([]float32, []float32) {
	lo := []float32{v[0], v[1], v[2]}
	hi := []float32{v[0], v[1], v[2]}
	for i := 0; i < len(v); i += 3 {
		for c := range 3 {
			lo[c], hi[c] = min(lo[c], v[i+c]), max(hi[c], v[i+c])
		}
	}
	return lo, hi
}

// glb packs the JSON document and binary buffer into a binary glTF file.
func glb(doc map[string]any, bin []byte) []byte {
	js, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	for len(js)%4 != 0 {
		js = append(js, ' ')
	}
	for len(bin)%4 != 0 {
		bin = append(bin, 0)
	}
	var out bytes.Buffer
	w := func(v ...uint32) { binary.Write(&out, binary.LittleEndian, v) }
	w(0x46546C67, 2, uint32(12+8+len(js)+8+len(bin)))
	w(uint32(len(js)), 0x4E4F534A)
	out.Write(js)
	w(uint32(len(bin)), 0x004E4942)
	out.Write(bin)
	return out.Bytes()
}
