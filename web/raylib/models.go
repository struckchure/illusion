//go:build js

package rl

import (
	"syscall/js"
	"unsafe"
)

// Meshes stay in raylib's heap, since DrawMesh needs raylib's own copy of
// the vertex arrays. The Go Mesh carries the counts and VaoID, which keys the
// C pointer here, plus Go copies of the positions, texcoords, normals and
// indices (physics colliders read those). Other vertex arrays are nil.
var meshes = map[uint32]uint32{}

// wMeshInfo matches WMeshInfo in glue.c.
type wMeshInfo struct {
	VertexCount, TriangleCount            int32
	VaoID                                 uint32
	Vertices, Texcoords, Normals, Indices uint32
}

// mirrorMesh builds the Go side of the mesh at p and registers it for DrawMesh.
func mirrorMesh(p uint32) Mesh {
	call("w_MeshInfo", p, out())
	info := result[wMeshInfo]()
	if info.VaoID == 0 {
		panic("rl: mesh upload failed (no vertex array object)")
	}
	n := int(info.VertexCount)
	m := Mesh{
		VertexCount:   info.VertexCount,
		TriangleCount: info.TriangleCount,
		Vertices:      mirror[float32](info.Vertices, 3*n),
		Texcoords:     mirror[float32](info.Texcoords, 2*n),
		Normals:       mirror[float32](info.Normals, 3*n),
		Indices:       mirror[uint16](info.Indices, 3*int(info.TriangleCount)),
		VaoID:         info.VaoID,
	}
	meshes[m.VaoID] = p
	return m
}

func keepMesh(p js.Value) Mesh { return mirrorMesh(uint32(p.Int())) }

func GenMeshCube(width, height, length float32) Mesh {
	return keepMesh(call("w_GenMeshCube", width, height, length))
}

func GenMeshPlane(width, length float32, resX, resZ int) Mesh {
	return keepMesh(call("w_GenMeshPlane", width, length, resX, resZ))
}

func GenMeshSphere(radius float32, rings, slices int) Mesh {
	return keepMesh(call("w_GenMeshSphere", radius, rings, slices))
}

func GenMeshCylinder(radius, height float32, slices int) Mesh {
	return keepMesh(call("w_GenMeshCylinder", radius, height, slices))
}

func GenMeshTorus(radius, size float32, radSeg, sides int) Mesh {
	return keepMesh(call("w_GenMeshTorus", radius, size, radSeg, sides))
}

func UnloadMesh(mesh *Mesh) {
	if p, ok := meshes[mesh.VaoID]; ok {
		call("w_UnloadMesh", p)
		delete(meshes, mesh.VaoID)
	}
}

func DrawMesh(mesh Mesh, material Material, transform Matrix) {
	p, ok := meshes[mesh.VaoID]
	if !ok {
		panic("rl: DrawMesh: only meshes from GenMesh* or LoadModel can be drawn on the web so far")
	}
	call("w_DrawMesh", p, argMaterial(material), arg(transform))
}

// wMaterial matches WMaterial in glue.c.
type wMaterial struct {
	Shader uint32
	Locs   [MaxShaderLocations]int32
	Maps   [MaxMaterialMaps]MaterialMap
	Params [4]float32
}

func argMaterial(m Material) uint32 {
	w := wMaterial{Shader: m.Shader.ID, Params: m.Params}
	if m.Shader.Locs != nil {
		copy(w.Locs[:], unsafe.Slice(m.Shader.Locs, MaxShaderLocations))
	} else {
		for i := range w.Locs {
			w.Locs[i] = -1
		}
	}
	if m.Maps != nil {
		copy(w.Maps[:], unsafe.Slice(m.Maps, MaxMaterialMaps))
	}
	return arg(w)
}

// LoadMaterialDefault returns a material whose maps and shader locations live
// in Go memory, so GetMap and UpdateLocation work as on desktop.
func LoadMaterialDefault() Material {
	call("w_LoadMaterialDefault", out())
	return materialOf(result[wMaterial]())
}

func materialOf(w wMaterial) Material {
	locs := make([]int32, MaxShaderLocations)
	copy(locs, w.Locs[:])
	maps := make([]MaterialMap, MaxMaterialMaps)
	copy(maps, w.Maps[:])
	return Material{Shader: Shader{ID: w.Shader, Locs: &locs[0]}, Maps: &maps[0], Params: w.Params}
}

func UnloadMaterial(material Material) { call("w_UnloadMaterial", argMaterial(material)) }

// Shaders. The web uses WebGL 2, so shaders are GLSL ES 3.00 ("#version 300
// es").

func LoadShaderFromMemory(vsCode string, fsCode string) Shader {
	id := call("w_LoadShaderFromMemory", argString(vsCode), argString(fsCode), out()).Int()
	locs := result[[MaxShaderLocations]int32]()
	return Shader{ID: uint32(id), Locs: &locs[0]}
}

func UnloadShader(shader Shader) { call("w_UnloadShader", shader.ID) }

func GetShaderLocation(shader Shader, uniformName string) int32 {
	return int32(call("rlGetLocationUniform", shader.ID, argString(uniformName)).Int())
}

func SetShaderValue(shader Shader, locIndex int32, value []float32, uniformType ShaderUniformDataType) {
	call("w_SetShaderValue", shader.ID, locIndex, argSlice(value), int32(uniformType))
}

// Models stay in raylib's heap too. The Go Model mirrors its meshes
// (registered for DrawMesh like generated ones), materials (whose maps and
// shader locations are Go memory, as with LoadMaterialDefault) and
// mesh-material table; models keys the C pointer by the Go meshes array.
var models = map[*Mesh]uint32{}

// wModelInfo matches WModelInfo in glue.c.
type wModelInfo struct {
	MeshCount, MaterialCount int32
	Transform                Matrix
	MeshMaterial             uint32
}

// LoadModel loads from raylib's virtual filesystem: files must be bundled
// with the page (web/build.sh -a).
func LoadModel(fileName string) Model {
	p := uint32(call("w_LoadModel", argString(fileName)).Int())
	call("w_ModelInfo", p, out())
	info := result[wModelInfo]()

	ms := make([]Mesh, max(1, info.MeshCount))
	for i := range info.MeshCount {
		ms[i] = mirrorMesh(uint32(call("w_ModelMesh", p, i).Int()))
	}
	mats := make([]Material, max(1, info.MaterialCount))
	for i := range info.MaterialCount {
		call("w_ModelMaterial", p, i, out())
		mats[i] = materialOf(result[wMaterial]())
	}
	model := Model{
		Transform:     info.Transform,
		MeshCount:     info.MeshCount,
		MaterialCount: info.MaterialCount,
		Meshes:        &ms[0],
		Materials:     &mats[0],
		MeshMaterial:  mirror[int32](info.MeshMaterial, int(info.MeshCount)),
	}
	models[model.Meshes] = p
	return model
}

func IsModelValid(model Model) bool {
	p, ok := models[model.Meshes]
	return ok && truthy(call("w_IsModelValid", p))
}

func GetModelBoundingBox(model Model) BoundingBox {
	p, ok := models[model.Meshes]
	if !ok {
		return BoundingBox{}
	}
	call("w_GetModelBoundingBox", p, out())
	return result[BoundingBox]()
}

// UnloadModel frees the model's meshes and material maps, like raylib's; it
// leaves textures and shaders loaded.
func UnloadModel(model Model) {
	p, ok := models[model.Meshes]
	if !ok {
		return
	}
	for _, m := range model.GetMeshes() {
		delete(meshes, m.VaoID)
	}
	delete(models, model.Meshes)
	call("w_UnloadModel", p)
}
