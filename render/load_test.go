package render

import (
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func TestModelLoaded(t *testing.T) {
	vbo := []uint32{7, 8, 9}
	uploaded := []rl.Mesh{{VertexCount: 3, VboID: &vbo[0]}}
	materials := []rl.Material{{}}
	meshMaterial := []int32{0}
	model := func(meshes []rl.Mesh) rl.Model {
		return rl.Model{
			MeshCount: int32(len(meshes)), MaterialCount: 1,
			Meshes: &meshes[0], Materials: &materials[0], MeshMaterial: &meshMaterial[0],
		}
	}

	if modelLoaded(rl.Model{}) {
		t.Error("an empty model (a failed load) counts as loaded")
	}
	if !modelLoaded(model(uploaded)) {
		t.Error("a model with uploaded meshes doesn't count as loaded")
	}
	// A skinned mesh: bone data present, bone buffers not uploaded, as raylib
	// leaves them without GPU skinning. rl.IsModelValid rejects this.
	bones := []uint8{0, 0, 0, 0}
	weights := []float32{1, 0, 0, 0}
	skinned := []rl.Mesh{{VertexCount: 1, BoneCount: 1, BoneIndices: &bones[0], BoneWeights: &weights[0], VboID: &vbo[0]}}
	if !modelLoaded(model(skinned)) {
		t.Error("a skinned model doesn't count as loaded")
	}
	notUploaded := []uint32{0}
	if modelLoaded(model([]rl.Mesh{{VertexCount: 3, VboID: &notUploaded[0]}})) {
		t.Error("a model whose mesh isn't on the GPU counts as loaded")
	}
}
