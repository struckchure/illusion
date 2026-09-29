//go:build js

package rl

import (
	"image/color"
	"math"
	"unsafe"
)

// Skeletal animation runs in raylib, as on desktop: UpdateModelAnimation
// computes the pose and bone matrices and skins the vertices on the CPU,
// uploading them to the GPU. The Go Model mirrors its skeleton, and its
// CurrentPose and BoneMatrices are re-read after every update, so game code
// can read bone transforms (to attach a sword to a hand, say). The animated
// vertices (Mesh.AnimVertices, AnimNormals) stay in raylib.

// wSkeletonInfo matches WSkeletonInfo in glue.c.
type wSkeletonInfo struct {
	BoneCount                                  int32
	Bones, BindPose, CurrentPose, BoneMatrices uint32
}

func mirrorSkeleton(m *Model, p uint32) {
	call("w_ModelSkeleton", p, out())
	s := result[wSkeletonInfo]()
	n := int(s.BoneCount)
	m.Skeleton = ModelSkeleton{
		BoneCount: s.BoneCount,
		Bones:     mirror[BoneInfo](s.Bones, n),
		BindPose:  mirror[Transform](s.BindPose, n),
	}
	m.CurrentPose = mirror[Transform](s.CurrentPose, n)
	m.BoneMatrices = mirror[Matrix](s.BoneMatrices, n)
}

// syncPose re-reads the model's current pose and bone matrices into its Go
// arrays after an animation update.
func syncPose(m Model, p uint32) {
	call("w_ModelSkeleton", p, out())
	s := result[wSkeletonInfo]()
	n := int(min(s.BoneCount, m.Skeleton.BoneCount))
	if m.CurrentPose != nil && s.CurrentPose != 0 {
		readHeap(s.CurrentPose, unsafe.Slice((*byte)(unsafe.Pointer(m.CurrentPose)), n*int(unsafe.Sizeof(Transform{}))))
	}
	if m.BoneMatrices != nil && s.BoneMatrices != 0 {
		readHeap(s.BoneMatrices, unsafe.Slice((*byte)(unsafe.Pointer(m.BoneMatrices)), n*int(unsafe.Sizeof(Matrix{}))))
	}
}

// Animations stay in raylib too. The Go ModelAnimation mirrors every
// keyframe pose (so GetFramePose works) and keys the C animation here by its
// KeyframePoses array; animSets remembers each loaded array for unloading.
var (
	anims    = map[*ModelAnimPose]uint32{}
	animSets = map[*ModelAnimPose]struct {
		p uint32
		n int
	}{}
)

// wModelAnimation is ModelAnimation in wasm32 C.
type wModelAnimation struct {
	Name                     [32]uint8
	BoneCount, KeyframeCount int32
	KeyframePoses            uint32
}

// LoadModelAnimations loads from raylib's virtual filesystem: files must be
// bundled with the page (web/build.sh -a).
func LoadModelAnimations(fileName string) []ModelAnimation {
	p := uint32(call("w_LoadModelAnimations", argString(fileName), out()).Int())
	n := int(result[int32]())
	if p == 0 || n == 0 {
		return nil
	}
	cs := make([]wModelAnimation, n)
	readHeap(p, unsafe.Slice((*byte)(unsafe.Pointer(&cs[0])), n*int(unsafe.Sizeof(cs[0]))))
	list := make([]ModelAnimation, n)
	for i, c := range cs {
		frames, bones := int(c.KeyframeCount), int(c.BoneCount)
		poses := make([]ModelAnimPose, max(1, frames)) // never empty, so it can key anims
		if framePtrs := mirror[uint32](c.KeyframePoses, frames); framePtrs != nil {
			for f, fp := range unsafe.Slice(framePtrs, frames) {
				poses[f] = mirror[Transform](fp, bones)
			}
		}
		list[i] = ModelAnimation{Name: c.Name, BoneCount: c.BoneCount, KeyframeCount: c.KeyframeCount, KeyframePoses: &poses[0]}
		anims[list[i].KeyframePoses] = p + uint32(i)*uint32(unsafe.Sizeof(c))
	}
	animSets[list[0].KeyframePoses] = struct {
		p uint32
		n int
	}{p, n}
	return list
}

// UnloadModelAnimations unloads a slice LoadModelAnimations returned.
func UnloadModelAnimations(animations []ModelAnimation) {
	if len(animations) == 0 {
		return
	}
	set, ok := animSets[animations[0].KeyframePoses]
	if !ok {
		return
	}
	delete(animSets, animations[0].KeyframePoses)
	for _, a := range animations {
		delete(anims, a.KeyframePoses)
	}
	call("w_UnloadModelAnimations", set.p, set.n)
}

func IsModelAnimationValid(model Model, anim ModelAnimation) bool {
	mp, ok1 := models[model.Meshes]
	ap, ok2 := anims[anim.KeyframePoses]
	return ok1 && ok2 && truthy(call("w_IsModelAnimationValid", mp, ap))
}

// UpdateModelAnimation poses the model at frame (interpolating between
// keyframes) and skins its meshes.
func UpdateModelAnimation(model Model, anim ModelAnimation, frame float32) {
	mp, ok1 := models[model.Meshes]
	ap, ok2 := anims[anim.KeyframePoses]
	if !ok1 || !ok2 {
		return
	}
	call("w_UpdateModelAnimation", mp, ap, frame)
	syncPose(model, mp)
}

// UpdateModelAnimationEx blends two animations, blend 0 being animA.
func UpdateModelAnimationEx(model Model, animA ModelAnimation, frameA float32, animB ModelAnimation, frameB, blend float32) {
	mp, ok1 := models[model.Meshes]
	a, ok2 := anims[animA.KeyframePoses]
	b, ok3 := anims[animB.KeyframePoses]
	if !ok1 || !ok2 || !ok3 {
		return
	}
	call("w_UpdateModelAnimationEx", mp, a, frameA, b, frameB, blend)
	syncPose(model, mp)
}

// LoadModelFromMesh makes a one-mesh model with the default material. The
// model owns the mesh: unload the model, not the mesh.
func LoadModelFromMesh(mesh Mesh) Model {
	p := uint32(call("w_LoadModelFromMesh", meshPtr(mesh, "LoadModelFromMesh")).Int())
	call("w_ModelMaterial", p, 0, out())
	ms := []Mesh{mesh}
	mats := []Material{materialOf(result[wMaterial]())}
	meshMaterial := []int32{0}
	model := Model{
		Transform:     MatrixIdentity(),
		MeshCount:     1,
		MaterialCount: 1,
		Meshes:        &ms[0],
		Materials:     &mats[0],
		MeshMaterial:  &meshMaterial[0],
	}
	models[model.Meshes] = p
	return model
}

// deg2rad is raylib's DEG2RAD (PI/180.0f in float32), which is slightly more
// precise than raylib-go's Deg2rad literal.
const deg2rad = float32(math.Pi) / 180

// DrawModel draws the model at position, scaled uniformly and tinted.
func DrawModel(model Model, position Vector3, scale float32, tint color.RGBA) {
	DrawModelEx(model, position, Vector3{Y: 1}, 0, Vector3{X: scale, Y: scale, Z: scale}, tint)
}

// DrawModelEx is raylib's DrawModelEx, run on the Go side so the model's Go
// materials (textures, shaders, colors set by game code) are the ones drawn.
func DrawModelEx(model Model, position Vector3, rotationAxis Vector3, rotationAngle float32, scale Vector3, tint color.RGBA) {
	matScale := MatrixScale(scale.X, scale.Y, scale.Z)
	matRotation := MatrixRotate(rotationAxis, rotationAngle*deg2rad)
	matTranslation := MatrixTranslate(position.X, position.Y, position.Z)
	transform := MatrixMultiply(model.Transform, MatrixMultiply(MatrixMultiply(matScale, matRotation), matTranslation))

	p := models[model.Meshes]
	mats := model.GetMaterials()
	for i, mesh := range model.GetMeshes() {
		mat := mats[0]
		if model.MeshMaterial != nil {
			mat = mats[unsafe.Slice(model.MeshMaterial, model.MeshCount)[i]]
		}
		diffuse := mat.GetMap(MapDiffuse)
		col := diffuse.Color
		diffuse.Color = color.RGBA{
			R: uint8(int(col.R) * int(tint.R) / 255),
			G: uint8(int(col.G) * int(tint.G) / 255),
			B: uint8(int(col.B) * int(tint.B) / 255),
			A: uint8(int(col.A) * int(tint.A) / 255),
		}
		// Shaders that skin on the GPU get the bone matrices, as in raylib.
		if mat.Shader.Locs != nil && model.BoneMatrices != nil && p != 0 {
			if loc := mat.Shader.GetLocation(ShaderLocVertexBonetransforms); loc != -1 {
				call("w_SetBoneMatrices", p, mat.Shader.ID, loc)
			}
		}
		DrawMesh(mesh, mat, transform)
		diffuse.Color = col
	}
}
