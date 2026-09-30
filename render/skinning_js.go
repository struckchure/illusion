//go:build js

package render

import rl "github.com/gen2brain/raylib-go/raylib"

// routeSkinnedNormals is done by web/raylib/glue.c in the browser, where the
// meshes live in raylib's wasm memory; see skinning.go.
func routeSkinnedNormals(rl.Model) (restore func()) { return func() {} }
