// Package render draws the world with raylib.
//
// Spawn an entity with [Camera3d] and a transform to look through it, and
// entities with [Mesh3d] (or [Model3d]), [MeshMaterial3d] and a transform to
// draw them. For 2D, use [Camera2d] with [Sprite] and [Text2d]. For
// immediate-mode drawing, add systems to the Render schedule in the [Draw3D],
// [DrawWorld2D] or [Draw2D] sets.
package render

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/struckchure/illusion/asset"
)

// Mesh is GPU geometry. Create one with a primitive constructor like
// [Cuboid], or wrap an rl.Mesh you built yourself, then store it in
// asset.Assets[Mesh].
type Mesh struct {
	rl.Mesh
}

// Cuboid is a box centered on the origin.
func Cuboid(width, height, length float32) Mesh {
	return Mesh{rl.GenMeshCube(width, height, length)}
}

// Sphere is a UV sphere centered on the origin.
func Sphere(radius float32) Mesh {
	return Mesh{rl.GenMeshSphere(radius, 32, 32)}
}

// Plane is a flat, upward-facing rectangle on the XZ plane.
func Plane(width, length float32) Mesh {
	return Mesh{rl.GenMeshPlane(width, length, 1, 1)}
}

// Cylinder is an upright cylinder whose base sits on the origin.
func Cylinder(radius, height float32) Mesh {
	return Mesh{rl.GenMeshCylinder(radius, height, 32)}
}

// Torus is a ring lying on the XY plane. majorRadius is the distance from the
// center to the middle of the tube, minorRadius the tube's radius (at least a
// tenth of majorRadius and at most equal to it).
func Torus(majorRadius, minorRadius float32) Mesh {
	// raylib builds a unit-radius torus with the given tube ratio, then scales
	// it by size/2.
	return Mesh{rl.GenMeshTorus(minorRadius/majorRadius, 2*majorRadius, 32, 32)}
}

// Texture is an image on the GPU. Load one with asset.Loader[Texture].
type Texture struct {
	rl.Texture2D
}

// Model is a set of meshes and materials loaded from a file (.obj, .gltf,
// .glb, .iqm, .vox, .m3d). Load one with asset.Loader[Model].
type Model struct {
	rl.Model

	// textures the file loaded itself; raylib's UnloadModel leaves them.
	ownTextures []rl.Texture2D
}

// StandardMaterial describes a surface.
type StandardMaterial struct {
	// BaseColor tints the surface.
	BaseColor color.RGBA
	// Texture is multiplied with BaseColor. The zero handle means none.
	Texture asset.Handle[Texture]
	// Unlit ignores lights and draws BaseColor as is.
	Unlit bool
}

// Mesh3d draws a mesh at the entity's GlobalTransform.
type Mesh3d struct {
	Mesh asset.Handle[Mesh]
}

// Model3d draws a model at the entity's GlobalTransform, with the model's own
// materials unless the entity also has a MeshMaterial3d.
type Model3d struct {
	Model asset.Handle[Model]
}

// MeshMaterial3d sets the material a Mesh3d is drawn with; without it the mesh
// is drawn white. On a Model3d it replaces every material in the model.
type MeshMaterial3d struct {
	Material asset.Handle[StandardMaterial]
}

// Camera3d renders the scene from the entity's GlobalTransform, looking along
// its Forward direction. If several cameras exist, the one with the highest
// Order is used.
type Camera3d struct {
	// Fovy is the vertical field of view in degrees; 0 means 45. For
	// orthographic cameras it is the view height in world units.
	Fovy         float32
	Orthographic bool
	Order        int
}

// DirectionalLight lights the scene from infinitely far away, like the sun,
// shining along the entity's Forward direction. Only the first one found is
// used.
type DirectionalLight struct {
	Color color.RGBA
}

// AmbientLight is a resource: light that reaches every surface equally.
type AmbientLight struct {
	Color      color.RGBA
	Brightness float32
}

// ClearColor is a resource: the background color.
type ClearColor struct {
	Color color.RGBA
}

// Hidden stops an entity from being drawn.
type Hidden struct{}
