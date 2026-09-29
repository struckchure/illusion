// Bouncing blobs in 2D: sprites, a generated sprite-sheet animation, a 2D
// camera, world-space text, gizmos, synthesized sound effects and the stats
// overlay. Every texture and sound is generated at startup.
//
// WASD pans, the mouse wheel zooms, Q/E rotate the camera, left click spawns
// a burst of blobs, F3 toggles the overlay, Escape quits.
package main

import (
	"fmt"
	"image/color"
	"math"
	"math/rand/v2"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/audio"
	"github.com/struckchure/illusion/diag"
	"github.com/struckchure/illusion/engine"
	"github.com/struckchure/illusion/input"
	"github.com/struckchure/illusion/render"
	"github.com/struckchure/illusion/transform"
	"github.com/struckchure/illusion/window"
)

const (
	arenaW, arenaH = 1600, 900
	startBlobs     = 300
	burstBlobs     = 40
	frameSize      = 32
	frameCount     = 6
)

// Blob bounces around the arena.
type Blob struct {
	Velocity rl.Vector2
	Radius   float32
}

// Sheet holds the generated assets.
type Sheet struct {
	Blob   asset.Handle[render.Texture]
	Frames []rl.Rectangle
	Blip   asset.Handle[audio.Sound]
}

// BlipCooldown limits how often the bounce sound plays.
type BlipCooldown struct{ Left float32 }

func main() {
	illusion.New().
		AddPlugins(engine.DefaultPlugins(engine.Config{
			Window: window.Config{Title: "Illusion: sprites", MSAA: true, Resizable: true},
		})).
		InsertResource(
			illusion.R(&render.ClearColor{Color: rl.NewColor(20, 22, 34, 255)}),
			illusion.R(&diag.Overlay{Visible: true}),
			illusion.R(&BlipCooldown{}),
		).
		AddSystems(illusion.Startup, illusion.Fn3(setup)).
		AddSystems(illusion.Update,
			illusion.Fn5(bounce),
			illusion.Fn3(moveCamera),
			illusion.Fn5(spawnOnClick),
			illusion.Fn1(drawArena),
		).
		AddSystems(illusion.Render, illusion.Fn1(hud).InSet(render.Draw2D)).
		Run()
}

func setup(
	cmd *illusion.Commands,
	textures *illusion.Res[asset.Assets[render.Texture]],
	sounds *illusion.Res[asset.Assets[audio.Sound]],
) {
	// A sprite sheet of a blob squashing and stretching, one frame per cell.
	img := rl.GenImageColor(frameSize*frameCount, frameSize, rl.Blank)
	for i := range frameCount {
		phase := math.Sin(float64(i) / frameCount * 2 * math.Pi)
		rx := int32(12 + 3*phase)
		ry := int32(12 - 3*phase)
		cx := int32(i*frameSize + frameSize/2)
		fillEllipse(img, cx, frameSize/2, rx, ry, rl.White)
		rl.ImageDrawCircle(img, cx-4, frameSize/2-3, 3, rl.NewColor(30, 30, 40, 255))
		rl.ImageDrawCircle(img, cx+4, frameSize/2-3, 3, rl.NewColor(30, 30, 40, 255))
	}
	sheet := &Sheet{
		Blob:   textures.Get().Add(render.TextureFromImage(img)),
		Frames: render.GridFrames(frameCount, 1, frameSize, frameSize),
		Blip:   sounds.Get().Add(audio.SoundFromSamples(audio.Tone(660, 0.08), audio.SampleRate)),
	}
	rl.UnloadImage(img)
	cmd.InsertResource(illusion.R(sheet))

	cmd.Spawn(illusion.C(render.Camera2d{Zoom: 0.7}), illusion.C(transform.FromXY(0, 0)))
	cmd.Spawn(
		illusion.C(render.Text2d{Text: "illusion 2D", Size: 64, Color: rl.NewColor(255, 255, 255, 60)}),
		illusion.C(transform.FromXYZ(0, 0, -1)), // behind the blobs
	)
	for range startBlobs {
		spawnBlob(cmd, sheet, rl.Vector2{
			X: (rand.Float32() - 0.5) * (arenaW - 100),
			Y: (rand.Float32() - 0.5) * (arenaH - 100),
		})
	}
}

// fillEllipse draws a filled ellipse row by row.
func fillEllipse(img *rl.Image, cx, cy, rx, ry int32, c color.RGBA) {
	for dy := -ry; dy <= ry; dy++ {
		half := int32(float64(rx) * math.Sqrt(1-float64(dy*dy)/float64(ry*ry)))
		rl.ImageDrawRectangle(img, cx-half, cy+dy, 2*half+1, 1, c)
	}
}

func spawnBlob(cmd *illusion.Commands, sheet *Sheet, at rl.Vector2) {
	angle := rand.Float64() * 2 * math.Pi
	speed := 60 + rand.Float32()*240
	scale := 0.6 + rand.Float32()*1.4
	hue := rand.Float32() * 360
	cmd.Spawn(
		illusion.C(Blob{
			Velocity: rl.Vector2{X: speed * float32(math.Cos(angle)), Y: speed * float32(math.Sin(angle))},
			Radius:   12 * scale,
		}),
		illusion.C(render.Sprite{Texture: sheet.Blob, Color: rl.ColorFromHSV(hue, 0.6, 1)}),
		illusion.C(render.SpriteAnimation{Frames: sheet.Frames, FPS: 6 + rand.Float32()*8}),
		// Random Z spreads blobs over draw layers; bigger ones in front.
		illusion.C(transform.FromXYZ(at.X, at.Y, scale).WithScale(scale)),
	)
}

// bounce moves blobs and reflects them off the arena walls with a blip.
func bounce(
	q *illusion.Query2[Blob, transform.Transform],
	t *illusion.Res[illusion.Time],
	sheet *illusion.Res[Sheet],
	sfx *audio.Audio,
	cooldown *illusion.Res[BlipCooldown],
) {
	dt := t.Get().DeltaSecs()
	cd := cooldown.Get()
	cd.Left -= dt
	q.Each(func(_ ecs.Entity, b *Blob, tr *transform.Transform) {
		p := &tr.Translation
		p.X += b.Velocity.X * dt
		p.Y += b.Velocity.Y * dt
		hit := false
		if limit := arenaW/2 - b.Radius; p.X < -limit || p.X > limit {
			p.X = max(-limit, min(limit, p.X))
			b.Velocity.X = -b.Velocity.X
			hit = true
		}
		if limit := arenaH/2 - b.Radius; p.Y < -limit || p.Y > limit {
			p.Y = max(-limit, min(limit, p.Y))
			b.Velocity.Y = -b.Velocity.Y
			hit = true
		}
		if hit && cd.Left <= 0 {
			cd.Left = 0.06
			speed := rl.Vector2Length(b.Velocity)
			sfx.PlayWith(sheet.Get().Blip, audio.Playback{
				Volume: 0.35,
				Pitch:  0.7 + speed/300,
				Pan:    p.X / (arenaW / 2),
			})
		}
	})
}

func moveCamera(
	q *illusion.Query2[render.Camera2d, transform.Transform],
	keys *illusion.Res[input.Keys],
	mouse *illusion.Res[input.Mouse],
) {
	k := keys.Get()
	q.Each(func(_ ecs.Entity, cam *render.Camera2d, tr *transform.Transform) {
		speed := 600 / cam.Zoom * rl.GetFrameTime()
		if k.Pressed(rl.KeyA) {
			tr.Translation.X -= speed
		}
		if k.Pressed(rl.KeyD) {
			tr.Translation.X += speed
		}
		if k.Pressed(rl.KeyW) {
			tr.Translation.Y -= speed
		}
		if k.Pressed(rl.KeyS) {
			tr.Translation.Y += speed
		}
		if k.Pressed(rl.KeyQ) {
			tr.RotateZ(-rl.GetFrameTime())
		}
		if k.Pressed(rl.KeyE) {
			tr.RotateZ(rl.GetFrameTime())
		}
		if w := mouse.Get().Wheel; w != 0 {
			cam.Zoom = max(0.2, min(4, cam.Zoom*float32(math.Pow(1.1, float64(w)))))
		}
	})
}

func spawnOnClick(
	buttons *illusion.Res[input.MouseButtons],
	mouse *illusion.Res[input.Mouse],
	view *illusion.Res[render.View2D],
	sheet *illusion.Res[Sheet],
	cmd *illusion.Commands,
) {
	if !buttons.Get().JustPressed(rl.MouseButtonLeft) || !view.Get().Active {
		return
	}
	at := view.Get().ScreenToWorld(mouse.Get().Position)
	for range burstBlobs {
		spawnBlob(cmd, sheet.Get(), at)
	}
}

func drawArena(g *diag.Gizmos) {
	g.Rect2D(rl.Vector2{}, rl.Vector2{X: arenaW, Y: arenaH}, 0, color.RGBA{R: 120, G: 140, B: 255, A: 255})
	g.Circle2D(rl.Vector2{}, 8, rl.Gray)
}

func hud(blobs *illusion.Query0Where[illusion.With[Blob]]) {
	rl.DrawText("WASD pan   wheel zoom   Q/E rotate   click spawn   F3 overlay", 10, 10, 20, rl.RayWhite)
	rl.DrawText(fmt.Sprintf("%d blobs", blobs.Count()), 10, 36, 20, rl.Gold)
}
