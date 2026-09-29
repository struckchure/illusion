package render

import (
	"errors"
	"path/filepath"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func loadTexture(path string) (Texture, error) {
	t := rl.LoadTexture(path)
	if !rl.IsTextureValid(t) {
		return Texture{}, errors.New("raylib could not load the texture")
	}
	return Texture{t}, nil
}

func unloadTexture(t *Texture) {
	rl.UnloadTexture(t.Texture2D)
}

func loadModel(path string) (Model, error) {
	if strings.EqualFold(filepath.Ext(path), ".obj") {
		safe, cleanup, err := sanitizeOBJ(path)
		if err != nil {
			return Model{}, err
		}
		defer cleanup()
		path = safe
	}
	m := rl.LoadModel(path)
	if !modelLoaded(m) {
		rl.UnloadModel(m)
		return Model{}, errors.New("raylib could not load the model")
	}
	model := Model{Model: m}
	defaultID := rl.GetTextureIdDefault()
	seen := map[uint32]bool{}
	for _, mat := range m.GetMaterials() {
		for i := int32(0); i < rl.MaxMaterialMaps; i++ {
			t := mat.GetMap(i).Texture
			if t.ID != 0 && t.ID != defaultID && !seen[t.ID] {
				seen[t.ID] = true
				model.ownTextures = append(model.ownTextures, t)
			}
		}
	}
	return model, nil
}

// modelLoaded reports whether rl.LoadModel succeeded: the model has meshes
// and materials, and every mesh's vertices are on the GPU. rl.IsModelValid
// can't be used: raylib 6.0 built without GPU skinning (the default) never
// uploads bone buffers, yet IsModelValid requires them, so it rejects every
// skinned model.
func modelLoaded(m rl.Model) bool {
	if m.MeshCount == 0 || m.MaterialCount == 0 || m.Meshes == nil || m.Materials == nil || m.MeshMaterial == nil {
		return false
	}
	for _, mesh := range m.GetMeshes() {
		if mesh.VboID == nil || *mesh.VboID == 0 {
			return false
		}
	}
	return true
}

func unloadModel(m *Model) {
	// UnloadModel frees meshes and material maps but not textures or shaders.
	rl.UnloadModel(m.Model)
	for _, t := range m.ownTextures {
		rl.UnloadTexture(t)
	}
}
