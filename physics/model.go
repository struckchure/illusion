package physics

import (
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Colliders built from loaded models. Pass the embedded raylib model, e.g.
// physics.ModelBox(model.Model) for a render.Model. They use the model's
// vertices as stored, ignoring model.Transform.

// ModelBox is a box around the model's bounds, offset to match where the
// model sits relative to its origin.
func ModelBox(m rl.Model) Collider {
	bb := rl.GetModelBoundingBox(m)
	size := rl.Vector3Subtract(bb.Max, bb.Min)
	center := rl.Vector3Scale(rl.Vector3Add(bb.Min, bb.Max), 0.5)
	c := Cuboid(size.X, size.Y, size.Z)
	if center != (rl.Vector3{}) {
		c = c.WithOffset(center)
	}
	return c
}

// ModelConvexHull is the convex hull of every vertex in the model. Use it for
// dynamic and kinematic bodies whose shape a box fits poorly.
func ModelConvexHull(m rl.Model) Collider {
	var points [][3]float32
	for _, mesh := range m.GetMeshes() {
		points = append(points, meshPoints(mesh)...)
	}
	return Collider{kind: shapeConvexHull, points: points}
}

// ModelTriMesh is the model's exact triangles, for static level geometry.
func ModelTriMesh(m rl.Model) Collider {
	var points [][3]float32
	var indices []uint32
	for _, mesh := range m.GetMeshes() {
		base := uint32(len(points))
		c := MeshCollider(mesh)
		points = append(points, c.points...)
		for _, i := range c.indices {
			indices = append(indices, base+i)
		}
	}
	return Collider{kind: shapeMesh, points: points, indices: indices}
}

func meshPoints(m rl.Mesh) [][3]float32 {
	n := int(m.VertexCount)
	raw := unsafe.Slice(m.Vertices, n*3)
	points := make([][3]float32, n)
	for i := range points {
		points[i] = [3]float32{raw[3*i], raw[3*i+1], raw[3*i+2]}
	}
	return points
}
