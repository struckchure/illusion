package render

import rl "github.com/gen2brain/raylib-go/raylib"

// MeshData is a mesh as NewMesh takes it: each vertex's position, and its
// normal and texture coordinates (either may be nil, for none), and the
// triangles, three indices each (nil for a vertex list, three to a
// triangle).
type MeshData struct {
	Positions []rl.Vector3
	Normals   []rl.Vector3
	Texcoords []rl.Vector2
	Indices   []uint16
}

// NewMesh makes a mesh of d and uploads it. Its arrays are raylib's, as a
// loaded or generated mesh's are, so it's freed as they are: with
// rl.UnloadMesh, as the Mesh asset store does when it's removed.
func NewMesh(d MeshData) rl.Mesh {
	m := rl.Mesh{VertexCount: int32(len(d.Positions))}
	if d.Indices != nil {
		m.TriangleCount = int32(len(d.Indices) / 3)
	} else {
		m.TriangleCount = m.VertexCount / 3
	}
	m.Vertices = (*float32)(meshArray(d.Positions))
	m.Normals = (*float32)(meshArray(d.Normals))
	m.Texcoords = (*float32)(meshArray(d.Texcoords))
	m.Indices = (*uint16)(meshArray(d.Indices))
	rl.UploadMesh(&m, false)
	return m
}
