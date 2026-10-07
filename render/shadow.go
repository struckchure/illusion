package render

import (
	"math"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/transform"
)

// The directional light's shadows: each frame what's round the view is drawn
// from the light into a depth map (an orthographic camera looking along the
// light), and the shaders compare each surface's depth from the light with
// the map's to tell whether something nearer the light is in the way.

// shadowUnit is the texture unit the shadow map is bound to while the scene
// is drawn: above those raylib binds a material's maps to.
const shadowUnit = 10

// rlgl's framebuffer attachment types.
const (
	attachDepth     = 100
	attachTexture2D = 100
)

// Shadows' defaults.
const (
	defaultShadowRange = 50
	defaultShadowBias  = 0.03
)

// shadowNear is the shadow map's near plane, in metres from its camera.
const shadowNear = 1

// shadowMap is the shadow map and how it was drawn this frame.
type shadowMap struct {
	target rl.RenderTexture2D
	size   int32
	on     bool // drawn this frame
	// viewProj takes a point in the world to the map's clip space.
	viewProj rl.Matrix
	// texel is a texel's width in metres, and bias the depth bias in the
	// map's depths (0 at its near plane, 1 at its far).
	texel, bias float32
	bounds      map[asset.Handle[Model]]sphere
}

// sphere is a model's bounds as a sphere about its middle, in its own frame.
type sphere struct {
	center rl.Vector3
	radius float32
}

// depthShader draws into the shadow map: depth only, cutting out what's
// less than half opaque, as the lit shaders do.
var depthShader = &Shader{Fragment: depthFragmentShader}

const depthFragmentShader = `
in vec2 fragTexCoord;
in vec4 fragColor;

uniform sampler2D texture0;
uniform vec4 colDiffuse;

out vec4 finalColor;

void main() {
    if ((texture(texture0, fragTexCoord) * colDiffuse * fragColor).a < 0.5) {
        discard;
    }
    finalColor = vec4(1.0);
}
`

// shadowCasters is drawShadows' view of what casts shadows: one parameter
// to stay within Fn8.
type shadowCasters struct {
	meshes       illusion.Query2Where[Mesh3d, transform.GlobalTransform, illusion.And[illusion.Without[Hidden], illusion.Without[NotShadowCaster]]]
	models       illusion.Query2Where[Model3d, transform.GlobalTransform, illusion.And[illusion.Without[Hidden], illusion.Without[NotShadowCaster]]]
	withMaterial illusion.Query1[MeshMaterial3d]
	players      illusion.Query1[AnimationPlayer]
	parts        illusion.Query1[ModelParts]
	meshStore    illusion.Res[asset.Assets[Mesh]]
	modelStore   illusion.Res[asset.Assets[Model]]
	animations   illusion.Res[asset.Assets[Animations]]
}

func (c *shadowCasters) InitParam(w *ecs.World) {
	c.meshes.InitParam(w)
	c.models.InitParam(w)
	c.withMaterial.InitParam(w)
	c.players.InitParam(w)
	c.parts.InitParam(w)
	c.meshStore.InitParam(w)
	c.modelStore.InitParam(w)
	c.animations.InitParam(w)
}

// shadowView is where the shadow map's camera looks from and how wide it
// sees.
type shadowView struct {
	camera rl.Camera3D
	// focus is the middle of the square that's shadowed; right and up are
	// the map's axes in the world.
	focus, right, up rl.Vector3
	half             float32
}

// placeShadow puts the shadow map's camera over what a camera at eye,
// looking along forward, sees: a square half wide round a point ahead of
// it, seen along dir (the way the light shines) from reach away. The
// square's middle is snapped to whole texels of a size-texel map, so
// shadows don't crawl as the camera moves.
func placeShadow(eye, forward, dir rl.Vector3, half, reach float32, size int32) shadowView {
	dir = rl.Vector3Normalize(dir)
	ahead := rl.Vector3{X: forward.X, Z: forward.Z}
	if rl.Vector3Length(ahead) > 1e-4 {
		ahead = rl.Vector3Normalize(ahead)
	}
	focus := rl.Vector3Add(eye, rl.Vector3Scale(ahead, half/2))

	// The axes raylib's MatrixLookAt gives the camera.
	up := rl.Vector3{Y: 1}
	if float32(math.Abs(float64(dir.Y))) > 0.99 {
		up = rl.Vector3{Z: 1}
	}
	back := rl.Vector3Negate(dir)
	right := rl.Vector3Normalize(rl.Vector3CrossProduct(up, back))
	top := rl.Vector3CrossProduct(back, right)

	texel := 2 * half / float32(size)
	snap := func(v float32) float32 { return float32(math.Round(float64(v/texel))) * texel }
	a, b := rl.Vector3DotProduct(focus, right), rl.Vector3DotProduct(focus, top)
	focus = rl.Vector3Add(focus, rl.Vector3Add(rl.Vector3Scale(right, snap(a)-a), rl.Vector3Scale(top, snap(b)-b)))

	return shadowView{
		camera: rl.Camera3D{
			Position:   rl.Vector3Subtract(focus, rl.Vector3Scale(dir, reach)),
			Target:     focus,
			Up:         up,
			Fovy:       2 * half,
			Projection: rl.CameraOrthographic,
		},
		focus: focus, right: right, up: top, half: half,
	}
}

// sees reports whether a sphere about c of radius r falls in the square
// (or towards or away from the light from it).
func (v shadowView) sees(c rl.Vector3, r float32) bool {
	d := rl.Vector3Subtract(c, v.focus)
	return float32(math.Abs(float64(rl.Vector3DotProduct(d, v.right)))) <= v.half+r &&
		float32(math.Abs(float64(rl.Vector3DotProduct(d, v.up)))) <= v.half+r
}

func drawShadows(
	res *illusion.Res[renderer],
	settings *illusion.Res[Shadows],
	cameras *illusion.Query2[Camera3d, transform.GlobalTransform],
	lights *illusion.Query2[DirectionalLight, transform.GlobalTransform],
	materials *illusion.Res[asset.Assets[StandardMaterial]],
	casters *shadowCasters,
) {
	r := res.Get()
	s := settings.Get()
	r.shadow.on = false
	if !r.ready || s.Size <= 0 {
		r.freeShadow()
		return
	}
	_, camTransform, ok := activeCamera(cameras)
	if !ok {
		return
	}
	_, _, sun, ok := firstLight(lights)
	if !ok {
		return
	}
	if r.shadow.size != s.Size {
		r.freeShadow()
		if !r.loadShadow(s.Size) {
			return
		}
	}
	half, bias := s.Range, s.Bias
	if half <= 0 {
		half = defaultShadowRange
	}
	if bias <= 0 {
		bias = defaultShadowBias
	}
	reach := s.Reach
	if reach <= 0 {
		reach = 4 * half
	}
	view := placeShadow(camTransform.Translation(), camTransform.Forward(), sun.Forward(), half, reach, s.Size)

	// The map mustn't be bound while it's drawn into.
	rl.ActiveTextureSlot(shadowUnit)
	rl.DisableTexture()
	rl.ActiveTextureSlot(0)

	// The map's depths run from its camera to past the far side of the
	// square: as few metres as will do, so they're fine-grained.
	far := reach + 2*half
	near0, far0 := rl.GetCullDistanceNear(), rl.GetCullDistanceFar()
	rl.SetClipPlanes(shadowNear, float64(far))
	rl.BeginTextureMode(r.shadow.target)
	rl.ClearBackground(rl.White)
	rl.BeginMode3D(view.camera)
	r.shadow.viewProj = rl.MatrixMultiply(rl.GetMatrixModelview(), rl.GetMatrixProjection())
	r.drawCasters(view, casters, materials.Get())
	rl.EndMode3D()
	rl.EndTextureMode()
	rl.SetClipPlanes(near0, far0)

	rl.ActiveTextureSlot(shadowUnit)
	rl.EnableTexture(r.shadow.target.Depth.ID)
	rl.ActiveTextureSlot(0)
	r.shadow.on = true
	r.shadow.texel = 2 * half / float32(s.Size)
	r.shadow.bias = bias / (far - shadowNear)
}

// drawCasters draws what casts shadows into the square view sees into the
// shadow map.
func (r *renderer) drawCasters(view shadowView, c *shadowCasters, materialStore *asset.Assets[StandardMaterial]) {
	depth := r.program(depthShader).shader
	meshStore, modelStore, animStore := c.meshStore.Get(), c.modelStore.Get(), c.animations.Get()

	meshes := c.meshes.Iter()
	for meshes.Next() {
		m, g := meshes.Get()
		mesh := meshStore.Get(m.Mesh)
		if mesh == nil {
			continue
		}
		mat := materialFor(&c.withMaterial, materialStore, meshes.Entity())
		if mat == nil {
			mat = &defaultMaterial
		}
		r.apply(mat)
		draw := r.material
		draw.Shader = depth
		rl.DrawMesh(mesh.Mesh, draw, g.Matrix)
	}

	r.draws = r.draws[:0]
	models := c.models.Iter()
	for models.Next() {
		m, g := models.Get()
		model := modelStore.Get(m.Model)
		if model == nil {
			continue
		}
		matrix := rl.MatrixMultiply(model.Transform, g.Matrix)
		if b := r.modelBounds(m.Model, model); !view.sees(rl.Vector3Transform(b.center, matrix), b.radius*matrixScale(matrix)) {
			continue
		}
		r.draws = append(r.draws, poseDraw(models.Entity(), model, matrix, &c.players, animStore))
	}
	byPose(r.draws)
	for _, d := range r.draws {
		e, model, matrix := d.entity, d.model, d.matrix
		if p, ok := c.players.Get(e); ok {
			if a := animStore.Get(p.Animations); a != nil {
				if r.posed == nil {
					r.posed = map[*rl.Mesh]appliedPose{}
				}
				// Cloth isn't stepped here: it's drawn as it was posed, and
				// the camera's pass moves it on.
				poseModel(model.Model, p, a, r.posed)
			}
		}
		override := materialFor(&c.withMaterial, materialStore, e)
		if override != nil {
			r.apply(override)
		}
		mp, _ := c.parts.Get(e)
		own := model.GetMaterials()
		meshMaterial := unsafe.Slice(model.MeshMaterial, model.MeshCount)
		for i, mesh := range model.GetMeshes() {
			hidden, swap := meshPart(mp, i)
			if hidden {
				continue
			}
			if override != nil {
				mat := r.material
				mat.Shader = depth
				rl.DrawMesh(mesh, mat, matrix)
				continue
			}
			mat := own[meshMaterial[i]]
			mat.Shader = depth
			if t := r.textures.Get(swap); t != nil {
				// Cut out by the texture this entity wears, as it's drawn.
				diffuse := mat.GetMap(rl.MapDiffuse)
				was := diffuse.Texture
				diffuse.Texture = t.Texture2D
				rl.DrawMesh(mesh, mat, matrix)
				diffuse.Texture = was
				continue
			}
			rl.DrawMesh(mesh, mat, matrix)
		}
	}
}

// modelBounds is a model's bounds, worked out the first time they're
// asked for.
func (r *renderer) modelBounds(h asset.Handle[Model], model *Model) sphere {
	if b, ok := r.shadow.bounds[h]; ok {
		return b
	}
	box := rl.GetModelBoundingBox(model.Model)
	b := sphere{
		center: rl.Vector3Scale(rl.Vector3Add(box.Min, box.Max), 0.5),
		radius: rl.Vector3Distance(box.Min, box.Max) / 2,
	}
	if r.shadow.bounds == nil {
		r.shadow.bounds = map[asset.Handle[Model]]sphere{}
	}
	r.shadow.bounds[h] = b
	return b
}

// matrixScale is the most m stretches anything, along any of its axes.
func matrixScale(m rl.Matrix) float32 {
	x := rl.Vector3Length(rl.Vector3{X: m.M0, Y: m.M1, Z: m.M2})
	y := rl.Vector3Length(rl.Vector3{X: m.M4, Y: m.M5, Z: m.M6})
	z := rl.Vector3Length(rl.Vector3{X: m.M8, Y: m.M9, Z: m.M10})
	return max(x, y, z)
}

// loadShadow makes a size by size shadow map, reporting whether the GPU
// took it.
func (r *renderer) loadShadow(size int32) bool {
	fbo := rl.LoadFramebuffer()
	if fbo == 0 {
		return false
	}
	depth := rl.LoadTextureDepth(size, size, false)
	rl.FramebufferAttach(fbo, depth, attachDepth, attachTexture2D, 0)
	if !rl.FramebufferComplete(fbo) {
		rl.UnloadFramebuffer(fbo)
		rl.UnloadTexture(rl.Texture2D{ID: depth})
		return false
	}
	tex := rl.Texture2D{ID: depth, Width: size, Height: size, Mipmaps: 1}
	r.shadow.target = rl.RenderTexture2D{ID: fbo, Texture: rl.Texture2D{Width: size, Height: size}, Depth: tex}
	r.shadow.size = size
	return true
}

// freeShadow lets the shadow map go, if there is one.
func (r *renderer) freeShadow() {
	if r.shadow.size == 0 {
		return
	}
	rl.ActiveTextureSlot(shadowUnit)
	rl.DisableTexture()
	rl.ActiveTextureSlot(0)
	rl.UnloadTexture(r.shadow.target.Depth)
	rl.UnloadFramebuffer(r.shadow.target.ID)
	r.shadow.target, r.shadow.size, r.shadow.on = rl.RenderTexture2D{}, 0, false
}

// sendShadow sends a program the shadow map: lightSpace, shadowMap, and
// shadowParams (on, a texel in the map, a texel in metres, the bias).
func (r *renderer) sendShadow(p *program) {
	params := p.loc("shadowParams")
	if params < 0 {
		return
	}
	if !r.shadow.on {
		rl.SetShaderValue(p.shader, params, []float32{0, 0, 0, 0}, rl.ShaderUniformVec4)
		return
	}
	size := float32(r.shadow.size)
	rl.SetShaderValue(p.shader, params, []float32{1, 1 / size, r.shadow.texel, r.shadow.bias}, rl.ShaderUniformVec4)
	if loc := p.loc("lightSpace"); loc >= 0 {
		rl.SetShaderValueMatrix(p.shader, loc, r.shadow.viewProj)
	}
	if loc := p.loc("shadowMap"); loc >= 0 {
		setInt(p.shader, loc, shadowUnit)
	}
}

// setInt sets an int (or sampler) uniform. raylib takes every uniform's
// value as floats, and reads an int's bits from them.
func setInt(shader rl.Shader, loc, v int32) {
	rl.SetShaderValue(shader, loc, []float32{math.Float32frombits(uint32(v))}, rl.ShaderUniformInt)
}
