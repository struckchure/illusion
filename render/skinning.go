//go:build !js

package render

import (
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Buffer slots in raylib's Mesh.vboId.
const (
	vboNormals = 2
	vboColors  = 3
)

// routeSkinnedNormals works around raylib 6.0's CPU skinning, which uploads
// the posed normals to vboId[SHADER_LOC_VERTEX_NORMAL], the colour slot (3),
// instead of the normal slot (2): the normals never reach the GPU, so posed
// meshes are lit as if they stood in their bind pose, and any vertex colours
// are overwritten (or, without them, OpenGL reports an error every frame).
// It points each skinned mesh's colour slot at its normal buffer for the
// update; call the returned function afterwards to put the slots back.
func routeSkinnedNormals(model rl.Model) (restore func()) {
	type swap struct {
		slot *uint32
		was  uint32
	}
	var swaps []swap
	for _, mesh := range model.GetMeshes() {
		if mesh.AnimNormals == nil || mesh.BoneWeights == nil || mesh.VboID == nil {
			continue
		}
		vbo := unsafe.Slice(mesh.VboID, vboColors+1)
		if vbo[vboNormals] == 0 {
			continue
		}
		swaps = append(swaps, swap{&vbo[vboColors], vbo[vboColors]})
		vbo[vboColors] = vbo[vboNormals]
	}
	return func() {
		for _, s := range swaps {
			*s.slot = s.was
		}
	}
}
