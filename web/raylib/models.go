//go:build js

package rl

import (
	"syscall/js"
	"unsafe"
)

// Meshes stay in raylib's heap, since DrawMesh needs raylib's own copy of
// the vertex arrays. The Go Mesh carries the counts, VaoID (which keys the C
// pointer here) and VboID. Generated and loaded meshes also get Go copies of
// their positions, texcoords, normals and indices (physics colliders read
// those); other vertex arrays are nil. A mesh built in Go keeps its own
// arrays: UploadMesh copies them to raylib.
var meshes = map[uint32]uint32{}

// meshVertexBuffers is MAX_MESH_VERTEX_BUFFERS in raylib's rmodels.c.
const meshVertexBuffers = 9

// wMeshInfo matches WMeshInfo in glue.c.
type wMeshInfo struct {
	VertexCount, TriangleCount            int32
	VaoID                                 uint32
	Vertices, Texcoords, Normals, Indices uint32
	BoneCount                             int32
	BoneIndices, BoneWeights              uint32
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
		BoneCount:     info.BoneCount,
		BoneIndices:   mirror[uint8](info.BoneIndices, 4*n),
		BoneWeights:   mirror[float32](info.BoneWeights, 4*n),
		VaoID:         info.VaoID,
	}
	register(&m, p)
	return m
}

// register records the C mesh at p for m and mirrors its VBO ids.
func register(m *Mesh, p uint32) {
	call("w_MeshVboIds", p, out(), meshVertexBuffers)
	vbo := result[[meshVertexBuffers]uint32]()
	m.VboID = &vbo[0]
	meshes[m.VaoID] = p
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

func GenMeshPoly(sides int, radius float32) Mesh {
	return keepMesh(call("w_GenMeshPoly", sides, radius))
}

func GenMeshHemiSphere(radius float32, rings, slices int) Mesh {
	return keepMesh(call("w_GenMeshHemiSphere", radius, rings, slices))
}

func GenMeshCone(radius, height float32, slices int) Mesh {
	return keepMesh(call("w_GenMeshCone", radius, height, slices))
}

func GenMeshKnot(radius, size float32, radSeg, sides int) Mesh {
	return keepMesh(call("w_GenMeshKnot", radius, size, radSeg, sides))
}

func genMeshFromImage(fn string, img Image, size Vector3) Mesh {
	data := malloc(imageBytes(&img))
	defer free(data)
	return keepMesh(call(fn, data, img.Width, img.Height, img.Mipmaps, int32(img.Format), arg(size)))
}

func GenMeshHeightmap(heightmap Image, size Vector3) Mesh {
	return genMeshFromImage("w_GenMeshHeightmap", heightmap, size)
}

func GenMeshCubicmap(cubicmap Image, cubeSize Vector3) Mesh {
	return genMeshFromImage("w_GenMeshCubicmap", cubicmap, cubeSize)
}

// wMeshArrays matches WMeshArrays in glue.c.
type wMeshArrays struct {
	VertexCount, TriangleCount                         int32
	Vertices, Texcoords, Texcoords2, Normals, Tangents uint32
	Colors, Indices                                    uint32
	BoneCount                                          int32
	BoneIndices, BoneWeights                           uint32
}

// copyArray copies n Ts from p (Go memory) into a new allocation in raylib's
// heap, which the uploaded mesh then owns. It returns 0 for a nil p.
func copyArray[T any](p *T, n int) uint32 {
	if p == nil || n <= 0 {
		return 0
	}
	return malloc(unsafe.Slice((*byte)(unsafe.Pointer(p)), uintptr(n)*unsafe.Sizeof(*p)))
}

// UploadMesh copies a mesh built in Go to raylib and uploads it to the GPU,
// setting VaoID and VboID. The Go arrays stay as they are; to change the mesh
// later, update them and call UpdateMeshBuffer. A mesh with BoneIndices and
// BoneWeights also gets the buffers CPU skinning writes (animated positions
// and normals stay in raylib).
func UploadMesh(mesh *Mesh, dynamic bool) {
	if _, ok := meshes[mesh.VaoID]; ok && mesh.VaoID != 0 {
		return // already uploaded; raylib ignores it too
	}
	n, t := int(mesh.VertexCount), int(mesh.TriangleCount)
	a := wMeshArrays{
		VertexCount:   mesh.VertexCount,
		TriangleCount: mesh.TriangleCount,
		Vertices:      copyArray(mesh.Vertices, 3*n),
		Texcoords:     copyArray(mesh.Texcoords, 2*n),
		Texcoords2:    copyArray(mesh.Texcoords2, 2*n),
		Normals:       copyArray(mesh.Normals, 3*n),
		Tangents:      copyArray(mesh.Tangents, 4*n),
		Colors:        copyArray(mesh.Colors, 4*n),
		Indices:       copyArray(mesh.Indices, 3*t),
		BoneCount:     mesh.BoneCount,
		BoneIndices:   copyArray(mesh.BoneIndices, 4*n),
		BoneWeights:   copyArray(mesh.BoneWeights, 4*n),
	}
	p := uint32(call("w_UploadMesh", arg(a), b2i(dynamic)).Int())
	call("w_MeshInfo", p, out())
	mesh.VaoID = result[wMeshInfo]().VaoID
	if mesh.VaoID == 0 {
		panic("rl: UploadMesh: mesh upload failed (no vertex array object)")
	}
	register(mesh, p)
}

// syncArray copies n Ts from the Go array g over the C array at c, when both
// exist.
func syncArray[T any](g *T, c uint32, n int) {
	if g != nil && c != 0 && n > 0 {
		writeHeap(c, unsafe.Slice((*byte)(unsafe.Pointer(g)), uintptr(n)*unsafe.Sizeof(*g)))
	}
}

// syncMesh copies m's Go vertex arrays over raylib's copies of them, before a
// C function that reads them (on desktop, raylib reads the Go arrays
// directly, so it sees changes made since the upload).
func syncMesh(m Mesh, p uint32) {
	call("w_MeshArrays", p, out())
	c := result[wMeshArrays]()
	n, t := int(c.VertexCount), int(c.TriangleCount)
	syncArray(m.Vertices, c.Vertices, 3*n)
	syncArray(m.Texcoords, c.Texcoords, 2*n)
	syncArray(m.Texcoords2, c.Texcoords2, 2*n)
	syncArray(m.Normals, c.Normals, 3*n)
	syncArray(m.Tangents, c.Tangents, 4*n)
	syncArray(m.Colors, c.Colors, 4*n)
	syncArray(m.Indices, c.Indices, 3*t)
	syncArray(m.BoneIndices, c.BoneIndices, 4*n)
	syncArray(m.BoneWeights, c.BoneWeights, 4*n)
}

// meshPtr returns the C mesh for m, panicking if it was never uploaded.
func meshPtr(m Mesh, fn string) uint32 {
	p, ok := meshes[m.VaoID]
	if !ok {
		panic("rl: " + fn + ": the mesh isn't uploaded (use GenMesh*, LoadModel or UploadMesh)")
	}
	return p
}

// UpdateMeshBuffer replaces part of one of the mesh's GPU buffers (index as in
// raylib: 0 positions, 1 texcoords, 2 normals, 3 colors, 4 tangents, ...).
func UpdateMeshBuffer(mesh Mesh, index int, data []byte, offset int) {
	p := meshPtr(mesh, "UpdateMeshBuffer")
	d := malloc(data)
	defer free(d)
	call("w_UpdateMeshBuffer", p, index, d, len(data), offset)
}

func GetMeshBoundingBox(mesh Mesh) BoundingBox {
	p := meshPtr(mesh, "GetMeshBoundingBox")
	syncMesh(mesh, p)
	call("w_GetMeshBoundingBox", p, out())
	return result[BoundingBox]()
}

// GenMeshTangents computes tangents and mirrors them into mesh.Tangents.
func GenMeshTangents(mesh *Mesh) {
	p := meshPtr(*mesh, "GenMeshTangents")
	syncMesh(*mesh, p)
	call("w_GenMeshTangents", p)
	mesh.Tangents = mirror[float32](uint32(call("w_MeshTangents", p).Int()), 4*int(mesh.VertexCount))
}

// ExportMesh writes to raylib's virtual filesystem, which lasts until the
// page closes.
func ExportMesh(mesh Mesh, fileName string) bool {
	p := meshPtr(mesh, "ExportMesh")
	syncMesh(mesh, p)
	return truthy(call("w_ExportMesh", p, argString(fileName)))
}

func UnloadMesh(mesh *Mesh) {
	if p, ok := meshes[mesh.VaoID]; ok {
		call("w_UnloadMesh", p)
		delete(meshes, mesh.VaoID)
	}
}

func DrawMesh(mesh Mesh, material Material, transform Matrix) {
	call("w_DrawMesh", meshPtr(mesh, "DrawMesh"), argMaterial(material), arg(transform))
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

func SetShaderValueV(shader Shader, locIndex int32, value []float32, uniformType ShaderUniformDataType, count int32) {
	call("w_SetShaderValueV", shader.ID, locIndex, argSlice(value), int32(uniformType), count)
}

func SetShaderValueMatrix(shader Shader, locIndex int32, mat Matrix) {
	call("w_SetShaderValueMatrix", shader.ID, locIndex, arg(mat))
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
	mirrorSkeleton(&model, p)
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
