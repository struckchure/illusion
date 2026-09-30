package render

import (
	"testing"

	"github.com/struckchure/illusion/asset"
)

func TestMeshPart(t *testing.T) {
	if hidden, tex := meshPart(nil, 3); hidden || !tex.IsZero() {
		t.Fatalf("no ModelParts: hidden %v texture %v, want neither", hidden, tex)
	}
	var store asset.Assets[Texture]
	skin := store.Add(Texture{})
	parts := &ModelParts{
		Hidden:  map[int]bool{1: true},
		Texture: map[int]asset.Handle[Texture]{2: skin},
	}
	if hidden, tex := meshPart(parts, 1); !hidden || !tex.IsZero() {
		t.Errorf("mesh 1: hidden %v texture %v, want hidden", hidden, tex)
	}
	if hidden, tex := meshPart(parts, 2); hidden || tex != skin {
		t.Errorf("mesh 2: hidden %v texture %v, want the skin", hidden, tex)
	}
	if hidden, tex := meshPart(parts, 0); hidden || !tex.IsZero() {
		t.Errorf("mesh 0: hidden %v texture %v, want neither", hidden, tex)
	}
}
