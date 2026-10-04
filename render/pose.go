package render

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"unsafe"
)

type poseScratch struct{ meshes []posedMesh }

// applyCustomPose uses the same skinning path as cloth on both desktop and
// browser. No temporary C animation or pointers into movable Go memory.
func applyCustomPose(model rl.Model, pose []rl.Transform, scratch *poseScratch) {
	bind := model.Skeleton.GetBindPose()
	if len(bind) != len(pose) || len(pose) == 0 || model.BoneMatrices == nil {
		return
	}
	bones := unsafe.Slice(model.BoneMatrices, len(pose))
	if model.CurrentPose != nil {
		copy(unsafe.Slice(model.CurrentPose, len(pose)), pose)
	}
	for i := range pose {
		bones[i] = rl.MatrixMultiply(rl.MatrixInvert(poseMatrix(bind[i])), poseMatrix(pose[i]))
	}
	meshes := model.GetMeshes()
	if len(scratch.meshes) != len(meshes) {
		scratch.meshes = make([]posedMesh, len(meshes))
	}
	for index, mesh := range meshes {
		n := int(mesh.VertexCount)
		if n == 0 || mesh.BoneIndices == nil || mesh.BoneWeights == nil {
			continue
		}
		ids, weights := unsafe.Slice(mesh.BoneIndices, 4*n), unsafe.Slice(mesh.BoneWeights, 4*n)
		buffers := &scratch.meshes[index]
		// The browser keeps raylib's animated vertex arrays in its C heap.
		// Skin into persistent Go buffers when they are not mirrored here.
		vertices := buffers.positions
		if mesh.AnimVertices != nil {
			vertices = unsafe.Slice((*rl.Vector3)(unsafe.Pointer(mesh.AnimVertices)), n)
		} else if len(vertices) != n {
			vertices = make([]rl.Vector3, n)
			buffers.positions = vertices
		}
		skin(vertices, meshVertices(mesh), ids, weights, bones)
		rl.UpdateMeshBuffer(mesh, 0, vectorBytes(vertices), 0)
		if mesh.Normals != nil {
			normals := unsafe.Slice((*rl.Vector3)(unsafe.Pointer(mesh.Normals)), n)
			posed := buffers.normals
			if mesh.AnimNormals != nil {
				posed = unsafe.Slice((*rl.Vector3)(unsafe.Pointer(mesh.AnimNormals)), n)
			} else if len(posed) != n {
				posed = make([]rl.Vector3, n)
				buffers.normals = posed
			}
			for i, normal := range normals {
				posed[i] = rl.Vector3Normalize(skinNormal(normal, ids[4*i:4*i+4], weights[4*i:4*i+4], bones))
			}
			rl.UpdateMeshBuffer(mesh, vboNormalSlot, vectorBytes(posed), 0)
		}
	}
}
