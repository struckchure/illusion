//go:build !js

package render

import (
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func TestRouteSkinnedNormals(t *testing.T) {
	skinnedVBOs := []uint32{1, 2, 3, 4, 5, 6, 7} // normals in 2, colours in 3
	plainVBOs := []uint32{8, 0, 9, 0, 0, 0, 10}
	normals, weights := []float32{0, 1, 0}, []float32{1, 0, 0, 0}
	meshes := []rl.Mesh{
		{VboID: &skinnedVBOs[0], AnimNormals: &normals[0], BoneWeights: &weights[0]},
		{VboID: &plainVBOs[0]}, // not skinned: left alone
	}
	model := rl.Model{MeshCount: int32(len(meshes)), Meshes: &meshes[0]}

	restore := routeSkinnedNormals(model)
	if skinnedVBOs[vboColors] != skinnedVBOs[vboNormals] {
		t.Fatalf("skinned mesh's colour slot = %d, want the normal buffer %d", skinnedVBOs[vboColors], skinnedVBOs[vboNormals])
	}
	if plainVBOs[vboColors] != 0 {
		t.Fatalf("unskinned mesh's colour slot changed to %d", plainVBOs[vboColors])
	}
	restore()
	if skinnedVBOs[vboColors] != 4 {
		t.Fatalf("after restore, colour slot = %d, want 4", skinnedVBOs[vboColors])
	}
}
