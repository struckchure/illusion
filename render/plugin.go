package render

import (
	"image/color"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/transform"
)

// Sets in the Render schedule, in the order they run.
const (
	// Begin starts the frame and clears the background.
	Begin illusion.SystemSet = "render.Begin"
	// Shadow draws the directional light's shadow map (see [Shadows]).
	Shadow illusion.SystemSet = "render.Shadow"
	// Begin3D enters 3D mode with the active camera.
	Begin3D illusion.SystemSet = "render.Begin3D"
	// Draw3D draws meshes. Add immediate-mode 3D drawing here (rl.DrawGrid,
	// rl.DrawCubeWires, ...). Skipped when there is no camera.
	Draw3D illusion.SystemSet = "render.Draw3D"
	// End3D leaves 3D mode.
	End3D illusion.SystemSet = "render.End3D"
	// Begin2D enters 2D mode with the active Camera2d.
	Begin2D illusion.SystemSet = "render.Begin2D"
	// DrawWorld2D draws sprites and 2D text through the 2D camera, over the
	// 3D scene. Add immediate-mode 2D world drawing here. Skipped when there
	// is no Camera2d.
	DrawWorld2D illusion.SystemSet = "render.DrawWorld2D"
	// End2D leaves 2D mode.
	End2D illusion.SystemSet = "render.End2D"
	// Draw2D is drawn over everything in screen space: HUDs, debug text.
	Draw2D illusion.SystemSet = "render.Draw2D"
	// End presents the frame.
	End illusion.SystemSet = "render.End"
)

// Plugin draws 3D cameras, meshes, models and lights, and 2D cameras,
// sprites and text. It needs the window plugin.
type Plugin struct{}

// Build implements [illusion.Plugin].
func (Plugin) Build(app *illusion.App) {
	asset.Register(app, func(m *Mesh) { rl.UnloadMesh(&m.Mesh) })
	asset.Register[StandardMaterial](app, nil)
	asset.RegisterLoader(app, loadTexture, unloadTexture)
	asset.RegisterLoader(app, loadModel, unloadModel)
	app.InitResource(
		illusion.R(&ClearColor{Color: color.RGBA{R: 24, G: 24, B: 28, A: 255}}),
		illusion.R(&AmbientLight{Color: rl.White, Brightness: 0.25}),
		illusion.R(&Shader{}),
		illusion.R(&Shadows{}),
	)
	app.InsertResource(illusion.R(&renderer{}), illusion.R(&View3D{}))

	app.AddSystems(illusion.PreStartup, illusion.Fn3(initRenderer).Named("render.init"))
	app.ConfigureSets(illusion.Render,
		Shadow.After(Begin),
		Begin3D.After(Shadow),
		Draw3D.After(Begin3D).RunIf(illusion.Cond1(cameraActive)),
		End3D.After(Draw3D),
		Begin2D.After(End3D),
		DrawWorld2D.After(Begin2D).RunIf(illusion.Cond1(camera2DActive)),
		End2D.After(DrawWorld2D),
		Draw2D.After(End2D),
		End.After(Draw2D),
	)
	app.AddSystems(illusion.Render,
		illusion.Fn1(beginFrame).InSet(Begin).Named("render.beginFrame"),
		illusion.Fn6(drawShadows).InSet(Shadow).Named("render.drawShadows"),
		illusion.Fn6(beginCamera).InSet(Begin3D).Named("render.beginCamera"),
		illusion.Fn6(drawMeshes).InSet(Draw3D).Named("render.drawMeshes"),
		illusion.Fn8(drawModels).InSet(Draw3D).Named("render.drawModels"),
		illusion.Fn1(endCamera).InSet(End3D).Named("render.endCamera"),
		illusion.Fn0(rl.EndDrawing).InSet(End).Named("render.endFrame"),
	)
	build2D(app)
	buildAnimation(app)
}

// Cleanup implements the optional plugin cleanup hook.
func (Plugin) Cleanup(app *illusion.App) {
	if r := ecs.GetResource[renderer](app.World); r != nil && r.ready {
		r.freeShadow()
		// UnloadMaterial unloads the textures in its maps; the diffuse map may
		// still hold the last drawn material's texture, which belongs to (and
		// has been unloaded with) the Texture store.
		r.material.GetMap(rl.MapDiffuse).Texture = r.defaultTexture
		rl.UnloadMaterial(r.material)
		for s, p := range r.programs {
			if s != r.shader {
				rl.UnloadShader(p.shader)
			}
		}
		r.programs = nil
		r.ready = false
	}
}

// renderer holds the GPU state shared by the render systems.
type renderer struct {
	ready          bool
	in3D           bool
	material       rl.Material
	defaultTexture rl.Texture2D

	textures *asset.Assets[Texture]

	// shader is the one everything is drawn with, and programs the compiled
	// shaders: that one and those of the passes.
	shader   *Shader
	programs map[*Shader]*program
	locUnlit int32
	// locEmissive is where the shader takes the emissive colour, and
	// emissive what it was last sent.
	locEmissive int32
	emissive    rl.Vector3

	// This frame's light and camera, for the shaders that take them; frame
	// counts frames, so each program is sent them once a frame.
	lightDir, lightColor, ambient, viewPos rl.Vector3
	frame                                  uint64

	// The point lights: all of them this frame, and those that light it.
	allLights, lights []pointLight

	shadow shadowMap

	posed map[*rl.Mesh]appliedPose // last pose applied to each model, by its meshes
	draws []posedDraw              // this pass's models, in the order they're drawn
}

func initRenderer(res *illusion.Res[renderer], textures *illusion.Res[asset.Assets[Texture]], shader *illusion.Res[Shader]) {
	r := res.Get()
	r.textures = textures.Get()
	r.shader = shader.Get()
	p := r.program(r.shader)
	r.material = rl.LoadMaterialDefault()
	r.material.Shader = p.shader
	r.defaultTexture = r.material.GetMap(rl.MapDiffuse).Texture
	r.locUnlit = p.loc("unlit")
	r.locEmissive = p.loc("emissive")
	r.ready = true
}

// program is a compiled Shader and its uniform locations, by name.
type program struct {
	shader rl.Shader
	locs   map[string]int32
	frame  uint64 // the frame it was last sent the light and camera
}

func (p *program) loc(name string) int32 {
	loc, ok := p.locs[name]
	if !ok {
		loc = rl.GetShaderLocation(p.shader, name)
		p.locs[name] = loc
	}
	return loc
}

// program returns s compiled, compiling it the first time.
func (r *renderer) program(s *Shader) *program {
	if p := r.programs[s]; p != nil {
		return p
	}
	vertex, fragment := s.Vertex, s.Fragment
	if vertex == "" {
		vertex = litVertexShader
	}
	if fragment == "" {
		fragment = litFragmentShader
	}
	p := &program{
		shader: rl.LoadShaderFromMemory(vertexHeader+"\n"+vertex, fragmentHeader+"\n"+fragment),
		locs:   map[string]int32{},
	}
	if r.programs == nil {
		r.programs = map[*Shader]*program{}
	}
	r.programs[s] = p
	return p
}

// uniformTypes are the types of a Shader's uniforms, by their length.
var uniformTypes = [...]rl.ShaderUniformDataType{1: rl.ShaderUniformFloat, rl.ShaderUniformVec2, rl.ShaderUniformVec3, rl.ShaderUniformVec4}

// use sends s this frame's lights, shadows and camera (the first time it's
// used in the frame) and its own uniforms, and returns it compiled.
func (r *renderer) use(s *Shader) rl.Shader {
	p := r.program(s)
	if p.frame != r.frame {
		p.frame = r.frame
		setVec3(p.shader, p.loc("lightDir"), r.lightDir)
		setVec3(p.shader, p.loc("lightColor"), r.lightColor)
		setVec3(p.shader, p.loc("ambient"), r.ambient)
		setVec3(p.shader, p.loc("viewPos"), r.viewPos)
		r.sendLights(p)
		r.sendShadow(p)
	}
	for name, v := range s.Uniforms {
		if loc := p.loc(name); loc >= 0 && len(v) >= 1 && len(v) <= 4 {
			rl.SetShaderValue(p.shader, loc, v, uniformTypes[len(v)])
		}
	}
	return p.shader
}

func cameraActive(r *illusion.Res[renderer]) bool {
	return r.Get().in3D
}

func beginFrame(clear *illusion.Res[ClearColor]) {
	rl.BeginDrawing()
	rl.ClearBackground(clear.Get().Color)
}

// View3D is a resource describing the active 3D camera this frame, for
// converting screen positions to rays (e.g. mouse picking).
type View3D struct {
	Camera rl.Camera3D
	Active bool
}

// ScreenToWorldRay returns the ray from the camera through a screen position.
func (v *View3D) ScreenToWorldRay(p rl.Vector2) rl.Ray { return rl.GetScreenToWorldRay(p, v.Camera) }

// WorldToScreen converts a world position to a screen position.
func (v *View3D) WorldToScreen(p rl.Vector3) rl.Vector2 { return rl.GetWorldToScreen(p, v.Camera) }

// activeCamera is the camera with the highest Order.
func activeCamera(cameras *illusion.Query2[Camera3d, transform.GlobalTransform]) (*Camera3d, *transform.GlobalTransform, bool) {
	var cam *Camera3d
	var camTransform *transform.GlobalTransform
	cameras.Each(func(_ ecs.Entity, c *Camera3d, g *transform.GlobalTransform) {
		if cam == nil || c.Order > cam.Order {
			cam, camTransform = c, g
		}
	})
	return cam, camTransform, cam != nil
}

func beginCamera(
	res *illusion.Res[renderer],
	cameras *illusion.Query2[Camera3d, transform.GlobalTransform],
	lights *illusion.Query2[DirectionalLight, transform.GlobalTransform],
	points *illusion.Query2[PointLight, transform.GlobalTransform],
	ambient *illusion.Res[AmbientLight],
	view *illusion.Res[View3D],
) {
	r := res.Get()
	r.in3D = false
	view.Get().Active = false

	cam, camTransform, ok := activeCamera(cameras)
	if !ok || !r.ready {
		return
	}

	lightDir := rl.Vector3{Y: -1}
	lightColor := rl.Vector3{}
	if _, light, g, ok := firstLight(lights); ok {
		lightDir = g.Forward()
		brightness := light.Brightness
		if brightness == 0 {
			brightness = 1
		}
		lightColor = rgb(light.Color, brightness)
	}
	a := ambient.Get()
	position := camTransform.Translation()
	r.frame++
	r.lightDir, r.lightColor, r.ambient, r.viewPos = lightDir, lightColor, rgb(a.Color, a.Brightness), position
	r.gatherLights(position, points)
	r.use(r.shader)

	fovy := cam.Fovy
	if fovy == 0 {
		fovy = 45
	}
	projection := rl.CameraPerspective
	if cam.Orthographic {
		projection = rl.CameraOrthographic
	}
	camera := rl.Camera3D{
		Position:   position,
		Target:     rl.Vector3Add(position, camTransform.Forward()),
		Up:         camTransform.Up(),
		Fovy:       fovy,
		Projection: projection,
	}
	rl.BeginMode3D(camera)
	r.in3D = true
	*view.Get() = View3D{Camera: camera, Active: true}
}

func firstLight(q *illusion.Query2[DirectionalLight, transform.GlobalTransform]) (ecs.Entity, *DirectionalLight, *transform.GlobalTransform, bool) {
	query := q.Iter()
	if !query.Next() {
		return ecs.Entity{}, nil, nil, false
	}
	light, g := query.Get()
	e := query.Entity()
	query.Close()
	return e, light, g, true
}

func drawMeshes(
	res *illusion.Res[renderer],
	meshes *illusion.Res[asset.Assets[Mesh]],
	materials *illusion.Res[asset.Assets[StandardMaterial]],
	q *illusion.Query2Where[Mesh3d, transform.GlobalTransform, illusion.And[illusion.Without[Hidden], illusion.Without[ShadowOnly]]],
	withMaterial *illusion.Query1[MeshMaterial3d],
	withPasses *illusion.Query1[Passes],
) {
	r := res.Get()
	meshStore, materialStore := meshes.Get(), materials.Get()

	query := q.Iter()
	for query.Next() {
		m, g := query.Get()
		mesh := meshStore.Get(m.Mesh)
		if mesh == nil {
			continue
		}
		mat := materialFor(withMaterial, materialStore, query.Entity())
		if mat == nil {
			mat = &defaultMaterial
		}
		r.apply(mat)
		rl.DrawMesh(mesh.Mesh, r.material, g.Matrix)

		if passes, ok := withPasses.Get(query.Entity()); ok {
			for _, pass := range *passes {
				// A Mesh3d is one mesh, index 0.
				if pass.Shader == nil || pass.Skip[0] {
					continue
				}
				if pass.CullFront {
					rl.SetCullFace(cullFaceFront)
				}
				passMaterial := r.material
				passMaterial.Shader = r.use(pass.Shader)
				rl.DrawMesh(mesh.Mesh, passMaterial, g.Matrix)
				if pass.CullFront {
					rl.SetCullFace(cullFaceBack)
				}
			}
		}
	}
}

func drawModels(
	res *illusion.Res[renderer],
	models *illusion.Res[asset.Assets[Model]],
	materials *illusion.Res[asset.Assets[StandardMaterial]],
	q *illusion.Query2Where[Model3d, transform.GlobalTransform, illusion.And[illusion.Without[Hidden], illusion.Without[ShadowOnly]]],
	withMaterial *illusion.Query1[MeshMaterial3d],
	players *illusion.Query1[AnimationPlayer],
	animations *illusion.Res[asset.Assets[Animations]],
	parts *modelParts,
) {
	r := res.Get()
	modelStore, materialStore, animStore := models.Get(), materials.Get(), animations.Get()
	textureStore := parts.textures.Get()
	dt := parts.time.Get().DeltaSecs()

	r.draws = r.draws[:0]
	query := q.Iter()
	for query.Next() {
		m, g := query.Get()
		model := modelStore.Get(m.Model)
		if model == nil {
			continue
		}
		r.draws = append(r.draws, poseDraw(query.Entity(), model, rl.MatrixMultiply(model.Transform, g.Matrix), players, animStore))
	}
	byPose(r.draws)
	for _, d := range r.draws {
		e, model, matrix := d.entity, d.model, d.matrix
		// The pose lives in the shared model, so apply this entity's right
		// before drawing it.
		if p, ok := players.Get(e); ok {
			if a := animStore.Get(p.Animations); a != nil {
				if r.posed == nil {
					r.posed = map[*rl.Mesh]appliedPose{}
				}
				if cloth, ok := parts.cloth.Get(e); ok && simulate(cloth, &model.Model, p, a, matrix, dt) {
					// The meshes now hold this entity's cloth: whoever
					// draws the model next has to pose it again.
					delete(r.posed, model.Meshes)
				} else {
					poseModel(model.Model, p, a, r.posed)
				}
			}
		}
		meshes := model.GetMeshes()
		mp, _ := parts.q.Get(e)
		override := materialFor(withMaterial, materialStore, e)
		own := model.GetMaterials()
		meshMaterial := unsafe.Slice(model.MeshMaterial, model.MeshCount)

		// draw draws the meshes that aren't hidden or skipped, with shader.
		draw := func(shader rl.Shader, skip map[int]bool) {
			main := shader.ID == r.material.Shader.ID
			if override != nil {
				r.apply(override)
			} else {
				r.setUnlit(false)
			}
			for i, mesh := range meshes {
				hidden, swap := meshPart(mp, i)
				if hidden || skip[i] {
					continue
				}
				if override != nil {
					mat := r.material
					mat.Shader = shader
					rl.DrawMesh(mesh, mat, matrix)
					continue
				}
				mat := own[meshMaterial[i]]
				if main {
					r.setEmissive(emission(mat))
				}
				mat.Shader = shader // light the model's own materials
				if t := textureStore.Get(swap); t != nil {
					// The maps are shared by every entity drawing this model, so
					// swap the texture for this draw only.
					diffuse := mat.GetMap(rl.MapDiffuse)
					own := diffuse.Texture
					diffuse.Texture = t.Texture2D
					rl.DrawMesh(mesh, mat, matrix)
					diffuse.Texture = own
					continue
				}
				rl.DrawMesh(mesh, mat, matrix)
			}
		}
		draw(r.material.Shader, nil)

		// The passes, while the meshes still hold this entity's pose.
		if passes, ok := parts.passes.Get(e); ok {
			for _, pass := range *passes {
				if pass.Shader == nil {
					continue
				}
				if pass.CullFront {
					rl.SetCullFace(cullFaceFront)
				}
				draw(r.use(pass.Shader), pass.Skip)
				if pass.CullFront {
					rl.SetCullFace(cullFaceBack)
				}
			}
		}
	}
}

// rlgl's face culling modes.
const (
	cullFaceFront = 0
	cullFaceBack  = 1
)

// modelParts is drawModels' view of ModelParts and the textures they swap
// in, of Cloth and the time it moves by, and of Passes: one parameter to
// stay within Fn8.
type modelParts struct {
	q        illusion.Query1[ModelParts]
	textures illusion.Res[asset.Assets[Texture]]
	cloth    illusion.Query1[Cloth]
	time     illusion.Res[illusion.Time]
	passes   illusion.Query1[Passes]
}

func (p *modelParts) InitParam(w *ecs.World) {
	p.q.InitParam(w)
	p.textures.InitParam(w)
	p.cloth.InitParam(w)
	p.time.InitParam(w)
	p.passes.InitParam(w)
}

// meshPart reports whether parts hides mesh i, and the texture it swaps in
// (the zero handle for none).
func meshPart(parts *ModelParts, i int) (hidden bool, texture asset.Handle[Texture]) {
	if parts == nil {
		return false, asset.Handle[Texture]{}
	}
	return parts.Hidden[i], parts.Texture[i]
}

var defaultMaterial = StandardMaterial{BaseColor: rl.White}

func materialFor(
	q *illusion.Query1[MeshMaterial3d],
	store *asset.Assets[StandardMaterial],
	e ecs.Entity,
) *StandardMaterial {
	if mm, ok := q.Get(e); ok {
		return store.Get(mm.Material)
	}
	return nil
}

// apply loads mat into the shared material used for drawing.
func (r *renderer) apply(mat *StandardMaterial) {
	diffuse := r.material.GetMap(rl.MapDiffuse)
	diffuse.Color = mat.BaseColor
	diffuse.Texture = r.defaultTexture
	if tex := r.textures.Get(mat.Texture); tex != nil {
		diffuse.Texture = tex.Texture2D
	}
	r.setUnlit(mat.Unlit)
	r.setEmissive(rgb(mat.Emissive, 1))
}

// emission is the light a model's own material gives off: its emission
// map's colour, which raylib's glTF loader sets when the material has an
// emissive texture.
func emission(mat rl.Material) rl.Vector3 {
	if mat.Maps == nil {
		return rl.Vector3{}
	}
	m := mat.GetMap(rl.MapEmission)
	if m.Texture.ID == 0 {
		return rl.Vector3{}
	}
	return rgb(m.Color, 1)
}

// setEmissive sends the shader the emissive colour of what's drawn next.
func (r *renderer) setEmissive(e rl.Vector3) {
	if e == r.emissive || r.locEmissive < 0 {
		return
	}
	r.emissive = e
	setVec3(r.material.Shader, r.locEmissive, e)
}

func (r *renderer) setUnlit(unlit bool) {
	v := float32(0)
	if unlit {
		v = 1
	}
	rl.SetShaderValue(r.material.Shader, r.locUnlit, []float32{v}, rl.ShaderUniformFloat)
}

func endCamera(res *illusion.Res[renderer]) {
	r := res.Get()
	if r.in3D {
		rl.EndMode3D()
		r.in3D = false
	}
}

func rgb(c color.RGBA, scale float32) rl.Vector3 {
	return rl.Vector3{
		X: float32(c.R) / 255 * scale,
		Y: float32(c.G) / 255 * scale,
		Z: float32(c.B) / 255 * scale,
	}
}

func setVec3(shader rl.Shader, loc int32, v rl.Vector3) {
	rl.SetShaderValue(shader, loc, []float32{v.X, v.Y, v.Z}, rl.ShaderUniformVec3)
}
