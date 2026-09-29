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
	)
	app.InsertResource(illusion.R(&renderer{}), illusion.R(&View3D{}))

	app.AddSystems(illusion.PreStartup, illusion.Fn2(initRenderer).Named("render.init"))
	app.ConfigureSets(illusion.Render,
		Begin3D.After(Begin),
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
		illusion.Fn5(beginCamera).InSet(Begin3D).Named("render.beginCamera"),
		illusion.Fn5(drawMeshes).InSet(Draw3D).Named("render.drawMeshes"),
		illusion.Fn5(drawModels).InSet(Draw3D).Named("render.drawModels"),
		illusion.Fn1(endCamera).InSet(End3D).Named("render.endCamera"),
		illusion.Fn0(rl.EndDrawing).InSet(End).Named("render.endFrame"),
	)
	build2D(app)
}

// Cleanup implements the optional plugin cleanup hook.
func (Plugin) Cleanup(app *illusion.App) {
	if r := ecs.GetResource[renderer](app.World); r != nil && r.ready {
		// UnloadMaterial unloads the textures in its maps; the diffuse map may
		// still hold the last drawn material's texture, which belongs to (and
		// has been unloaded with) the Texture store.
		r.material.GetMap(rl.MapDiffuse).Texture = r.defaultTexture
		rl.UnloadMaterial(r.material)
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

	locLightDir   int32
	locLightColor int32
	locAmbient    int32
	locUnlit      int32
}

func initRenderer(res *illusion.Res[renderer], textures *illusion.Res[asset.Assets[Texture]]) {
	r := res.Get()
	r.textures = textures.Get()
	shader := rl.LoadShaderFromMemory(litVertexShader, litFragmentShader)
	r.material = rl.LoadMaterialDefault()
	r.material.Shader = shader
	r.defaultTexture = r.material.GetMap(rl.MapDiffuse).Texture
	r.locLightDir = rl.GetShaderLocation(shader, "lightDir")
	r.locLightColor = rl.GetShaderLocation(shader, "lightColor")
	r.locAmbient = rl.GetShaderLocation(shader, "ambient")
	r.locUnlit = rl.GetShaderLocation(shader, "unlit")
	r.ready = true
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

func beginCamera(
	res *illusion.Res[renderer],
	cameras *illusion.Query2[Camera3d, transform.GlobalTransform],
	lights *illusion.Query2[DirectionalLight, transform.GlobalTransform],
	ambient *illusion.Res[AmbientLight],
	view *illusion.Res[View3D],
) {
	r := res.Get()
	r.in3D = false
	view.Get().Active = false

	var cam *Camera3d
	var camTransform *transform.GlobalTransform
	cameras.Each(func(_ ecs.Entity, c *Camera3d, g *transform.GlobalTransform) {
		if cam == nil || c.Order > cam.Order {
			cam, camTransform = c, g
		}
	})
	if cam == nil || !r.ready {
		return
	}

	lightDir := rl.Vector3{Y: -1}
	lightColor := rl.Vector3{}
	if _, light, g, ok := firstLight(lights); ok {
		lightDir = g.Forward()
		lightColor = rgb(light.Color, 1)
	}
	a := ambient.Get()
	shader := r.material.Shader
	setVec3(shader, r.locLightDir, lightDir)
	setVec3(shader, r.locLightColor, lightColor)
	setVec3(shader, r.locAmbient, rgb(a.Color, a.Brightness))

	position := camTransform.Translation()
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
	q *illusion.Query2Where[Mesh3d, transform.GlobalTransform, illusion.Without[Hidden]],
	withMaterial *illusion.Query1[MeshMaterial3d],
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
	}
}

func drawModels(
	res *illusion.Res[renderer],
	models *illusion.Res[asset.Assets[Model]],
	materials *illusion.Res[asset.Assets[StandardMaterial]],
	q *illusion.Query2Where[Model3d, transform.GlobalTransform, illusion.Without[Hidden]],
	withMaterial *illusion.Query1[MeshMaterial3d],
) {
	r := res.Get()
	modelStore, materialStore := models.Get(), materials.Get()

	query := q.Iter()
	for query.Next() {
		m, g := query.Get()
		model := modelStore.Get(m.Model)
		if model == nil {
			continue
		}
		matrix := rl.MatrixMultiply(model.Transform, g.Matrix)
		meshes := model.GetMeshes()

		if override := materialFor(withMaterial, materialStore, query.Entity()); override != nil {
			r.apply(override)
			for _, mesh := range meshes {
				rl.DrawMesh(mesh, r.material, matrix)
			}
			continue
		}

		r.setUnlit(false)
		own := model.GetMaterials()
		meshMaterial := unsafe.Slice(model.MeshMaterial, model.MeshCount)
		for i, mesh := range meshes {
			mat := own[meshMaterial[i]]
			mat.Shader = r.material.Shader // light the model's own materials
			rl.DrawMesh(mesh, mat, matrix)
		}
	}
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
