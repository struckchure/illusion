package render

import (
	"cmp"
	"image/color"
	"math"
	"slices"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/transform"
)

// Camera2d views the 2D world. The point at its entity's translation (X, Y)
// appears at the center of the screen, and rotating the entity around Z
// rotates the view.
//
// 2D coordinates follow raylib: +X is right, +Y is down, and one unit is one
// pixel at zoom 1. If several 2D cameras exist, the highest Order wins.
type Camera2d struct {
	// Zoom scales the view; 0 means 1.
	Zoom  float32
	Order int
}

// Sprite draws a texture (or a solid rectangle) at the entity's
// GlobalTransform in the 2D world. Sprites with a larger translation Z are
// drawn on top.
type Sprite struct {
	// Texture is the image to draw. The zero handle draws a solid rectangle.
	Texture asset.Handle[Texture]
	// Color tints the texture, or fills the rectangle. The zero color means
	// white.
	Color color.RGBA
	// Size is the size in world units before scaling. Zero means the size of
	// Source (or of the whole texture); for a solid rectangle it means 1x1,
	// so Transform.Scale sets the size.
	Size rl.Vector2
	// Source is the part of the texture to draw. The zero rectangle means
	// the whole texture.
	Source rl.Rectangle
	// Anchor is the pivot relative to the center, from -0.5 to 0.5 on each
	// axis: {0, 0} is the center, {-0.5, -0.5} the top-left corner.
	Anchor rl.Vector2
	// FlipX and FlipY mirror the texture.
	FlipX, FlipY bool
}

// Text2d draws text centered on the entity's GlobalTransform in the 2D world.
type Text2d struct {
	Text string
	// Size is the font height; 0 means 20.
	Size float32
	// Color is the text color; the zero color means white.
	Color color.RGBA
}

// SpriteAnimation flips through frames of a sprite sheet by setting the
// Sprite's Source. Build frames with [GridFrames].
type SpriteAnimation struct {
	Frames []rl.Rectangle
	// FPS is how many frames show per second.
	FPS float32
	// Once stops on the last frame instead of looping.
	Once bool

	frame   int
	elapsed float32
}

// Frame returns the index of the frame being shown.
func (a *SpriteAnimation) Frame() int { return a.frame }

// Finished reports whether a Once animation reached its last frame.
func (a *SpriteAnimation) Finished() bool { return a.Once && a.frame == len(a.Frames)-1 }

// GridFrames cuts a sprite sheet into frames of frameWidth x frameHeight
// pixels, left to right then top to bottom.
func GridFrames(columns, rows, frameWidth, frameHeight int) []rl.Rectangle {
	frames := make([]rl.Rectangle, 0, columns*rows)
	for y := range rows {
		for x := range columns {
			frames = append(frames, rl.Rectangle{
				X: float32(x * frameWidth), Y: float32(y * frameHeight),
				Width: float32(frameWidth), Height: float32(frameHeight),
			})
		}
	}
	return frames
}

// TextureFromImage uploads an image, e.g. one generated with rl.GenImage*.
// Store the result in asset.Assets[Texture].
func TextureFromImage(img *rl.Image) Texture {
	return Texture{rl.LoadTextureFromImage(img)}
}

// View2D is a resource describing the active 2D camera this frame, for
// converting between screen and world coordinates (e.g. mouse picking).
type View2D struct {
	Camera rl.Camera2D
	Active bool
}

// ScreenToWorld converts a screen position (like the mouse position) to 2D
// world coordinates.
func (v *View2D) ScreenToWorld(p rl.Vector2) rl.Vector2 { return rl.GetScreenToWorld2D(p, v.Camera) }

// WorldToScreen converts 2D world coordinates to a screen position.
func (v *View2D) WorldToScreen(p rl.Vector2) rl.Vector2 { return rl.GetWorldToScreen2D(p, v.Camera) }

func build2D(app *illusion.App) {
	app.InsertResource(illusion.R(&View2D{}))
	app.AddSystems(illusion.PostUpdate,
		illusion.Fn2(animateSprites).Before(transform.Propagate).Named("render.animateSprites"))
	app.AddSystems(illusion.Render,
		illusion.Fn2(beginCamera2D).InSet(Begin2D).Named("render.beginCamera2D"),
		illusion.Fn4(drawSprites).InSet(DrawWorld2D).Named("render.drawSprites"),
		illusion.Fn1(endCamera2D).InSet(End2D).Named("render.endCamera2D"),
	)
}

func animateSprites(q *illusion.Query2[SpriteAnimation, Sprite], t *illusion.Res[illusion.Time]) {
	dt := t.Get().DeltaSecs()
	q.Each(func(_ ecs.Entity, a *SpriteAnimation, s *Sprite) {
		if len(a.Frames) == 0 {
			return
		}
		if a.FPS > 0 && !a.Finished() {
			a.elapsed += dt
			step := 1 / a.FPS
			for a.elapsed >= step {
				a.elapsed -= step
				switch {
				case a.frame < len(a.Frames)-1:
					a.frame++
				case !a.Once:
					a.frame = 0
				}
			}
		}
		a.frame = min(a.frame, len(a.Frames)-1)
		s.Source = a.Frames[a.frame]
	})
}

func camera2DActive(v *illusion.Res[View2D]) bool { return v.Get().Active }

func beginCamera2D(cameras *illusion.Query2[Camera2d, transform.GlobalTransform], view *illusion.Res[View2D]) {
	v := view.Get()
	v.Active = false

	var cam *Camera2d
	var g *transform.GlobalTransform
	cameras.Each(func(_ ecs.Entity, c *Camera2d, gt *transform.GlobalTransform) {
		if cam == nil || c.Order > cam.Order {
			cam, g = c, gt
		}
	})
	if cam == nil {
		return
	}
	zoom := cam.Zoom
	if zoom == 0 {
		zoom = 1
	}
	pos, angle, _ := planar(g)
	v.Camera = rl.Camera2D{
		Offset:   rl.Vector2{X: float32(rl.GetScreenWidth()) / 2, Y: float32(rl.GetScreenHeight()) / 2},
		Target:   pos,
		Rotation: -angle * rl.Rad2deg, // the view turns opposite to the camera
		Zoom:     zoom,
	}
	rl.BeginMode2D(v.Camera)
	v.Active = true
}

func endCamera2D(view *illusion.Res[View2D]) {
	if view.Get().Active {
		rl.EndMode2D()
	}
}

// drawItem is one sprite or text to draw, sorted by depth.
type drawItem struct {
	z      float32
	sprite *Sprite
	text   *Text2d
	g      *transform.GlobalTransform
}

func drawSprites(
	sprites *illusion.Query2Where[Sprite, transform.GlobalTransform, illusion.Without[Hidden]],
	texts *illusion.Query2Where[Text2d, transform.GlobalTransform, illusion.Without[Hidden]],
	textures *illusion.Res[asset.Assets[Texture]],
	items *illusion.Local[[]drawItem],
) {
	list := (*items.Get())[:0]
	sprites.Each(func(_ ecs.Entity, s *Sprite, g *transform.GlobalTransform) {
		list = append(list, drawItem{z: g.Matrix.M14, sprite: s, g: g})
	})
	texts.Each(func(_ ecs.Entity, t *Text2d, g *transform.GlobalTransform) {
		list = append(list, drawItem{z: g.Matrix.M14, text: t, g: g})
	})
	slices.SortStableFunc(list, func(a, b drawItem) int { return cmp.Compare(a.z, b.z) })

	store := textures.Get()
	for _, it := range list {
		if it.sprite != nil {
			drawSprite(it.sprite, it.g, store)
		} else {
			drawText(it.text, it.g)
		}
	}
	clear(list) // don't keep pointers into component storage between frames
	*items.Get() = list[:0]
}

func drawSprite(s *Sprite, g *transform.GlobalTransform, store *asset.Assets[Texture]) {
	pos, angle, scale := planar(g)
	tint := orWhite(s.Color)
	tex := store.Get(s.Texture)

	size := s.Size
	src := s.Source
	if tex != nil && src.Width == 0 && src.Height == 0 {
		src = rl.Rectangle{Width: float32(tex.Width), Height: float32(tex.Height)}
	}
	if size.X == 0 && size.Y == 0 {
		size = rl.Vector2{X: src.Width, Y: src.Height}
		if tex == nil {
			size = rl.Vector2{X: 1, Y: 1}
		}
	}
	w, h := size.X*scale.X, size.Y*scale.Y
	dst := rl.Rectangle{X: pos.X, Y: pos.Y, Width: w, Height: h}
	origin := rl.Vector2{X: (0.5 + s.Anchor.X) * w, Y: (0.5 + s.Anchor.Y) * h}
	degrees := angle * rl.Rad2deg

	if tex == nil {
		rl.DrawRectanglePro(dst, origin, degrees, tint)
		return
	}
	if s.FlipX {
		src.Width = -src.Width
	}
	if s.FlipY {
		src.Height = -src.Height
	}
	rl.DrawTexturePro(tex.Texture2D, src, dst, origin, degrees, tint)
}

func drawText(t *Text2d, g *transform.GlobalTransform) {
	pos, angle, scale := planar(g)
	size := t.Size
	if size == 0 {
		size = 20
	}
	size *= scale.Y
	font := rl.GetFontDefault()
	spacing := size / 10
	measured := rl.MeasureTextEx(font, t.Text, size, spacing)
	origin := rl.Vector2{X: measured.X / 2, Y: measured.Y / 2}
	rl.DrawTextPro(font, t.Text, pos, origin, angle*rl.Rad2deg, size, spacing, orWhite(t.Color))
}

// planar extracts the 2D position, rotation around Z (radians, clockwise on
// screen) and XY scale from a world matrix.
func planar(g *transform.GlobalTransform) (pos rl.Vector2, angle float32, scale rl.Vector2) {
	m := g.Matrix
	pos = rl.Vector2{X: m.M12, Y: m.M13}
	angle = float32(math.Atan2(float64(m.M1), float64(m.M0)))
	scale = rl.Vector2{
		X: float32(math.Hypot(float64(m.M0), float64(m.M1))),
		Y: float32(math.Hypot(float64(m.M4), float64(m.M5))),
	}
	return pos, angle, scale
}

func orWhite(c color.RGBA) color.RGBA {
	if c == (color.RGBA{}) {
		return rl.White
	}
	return c
}
