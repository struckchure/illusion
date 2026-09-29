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
	if !rl.IsModelValid(m) {
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

func unloadModel(m *Model) {
	// UnloadModel frees meshes and material maps but not textures or shaders.
	rl.UnloadModel(m.Model)
	for _, t := range m.ownTextures {
		rl.UnloadTexture(t)
	}
}
