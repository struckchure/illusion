// Package input exposes keyboard and mouse state as resources, sampled once
// per frame in PreUpdate.
package input

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/struckchure/illusion"
)

// Key is a keyboard key. Use raylib's constants: rl.KeyW, rl.KeySpace, ...
type Key int32

// MouseButton is a mouse button: rl.MouseButtonLeft, ...
type MouseButton = rl.MouseButton

// ButtonInput tracks which buttons are held, and which changed this frame.
type ButtonInput[T comparable] struct {
	pressed      map[T]struct{}
	justPressed  map[T]struct{}
	justReleased map[T]struct{}
}

// NewButtonInput returns an empty ButtonInput.
func NewButtonInput[T comparable]() *ButtonInput[T] {
	return &ButtonInput[T]{
		pressed:      map[T]struct{}{},
		justPressed:  map[T]struct{}{},
		justReleased: map[T]struct{}{},
	}
}

// Pressed reports whether b is held down.
func (in *ButtonInput[T]) Pressed(b T) bool { return has(in.pressed, b) }

// JustPressed reports whether b went down this frame.
func (in *ButtonInput[T]) JustPressed(b T) bool { return has(in.justPressed, b) }

// JustReleased reports whether b went up this frame.
func (in *ButtonInput[T]) JustReleased(b T) bool { return has(in.justReleased, b) }

// AnyPressed reports whether any of bs is held down.
func (in *ButtonInput[T]) AnyPressed(bs ...T) bool {
	for _, b := range bs {
		if in.Pressed(b) {
			return true
		}
	}
	return false
}

// AnyJustPressed reports whether any of bs went down this frame.
func (in *ButtonInput[T]) AnyJustPressed(bs ...T) bool {
	for _, b := range bs {
		if in.JustPressed(b) {
			return true
		}
	}
	return false
}

// Press records b going down. The input plugin calls it; tests and replays can
// too.
func (in *ButtonInput[T]) Press(b T) {
	if !has(in.pressed, b) {
		in.pressed[b] = struct{}{}
		in.justPressed[b] = struct{}{}
	}
}

// Release records b going up.
func (in *ButtonInput[T]) Release(b T) {
	if has(in.pressed, b) {
		delete(in.pressed, b)
		in.justReleased[b] = struct{}{}
	}
}

// Clear forgets this frame's changes. The plugin calls it before sampling.
func (in *ButtonInput[T]) Clear() {
	clear(in.justPressed)
	clear(in.justReleased)
}

func has[T comparable](m map[T]struct{}, b T) bool {
	_, ok := m[b]
	return ok
}

// Keys is the keyboard state resource.
type Keys = ButtonInput[Key]

// MouseButtons is the mouse button state resource.
type MouseButtons = ButtonInput[MouseButton]

// Mouse is the cursor and wheel state resource.
type Mouse struct {
	// Position is the cursor position in window coordinates.
	Position rl.Vector2
	// Delta is how far the cursor moved this frame.
	Delta rl.Vector2
	// Wheel is how far the wheel scrolled this frame.
	Wheel float32
}

// Sample is the set that reads input from raylib. Systems in PreUpdate that
// read input should run after it.
const Sample illusion.SystemSet = "input.Sample"

// Settings is a resource that controls where input comes from.
type Settings struct {
	// Manual stops the plugin from reading the keyboard and mouse. Code then
	// drives Keys, MouseButtons and Mouse itself (with Press and Release),
	// e.g. for tests, replays or bots. The plugin still clears the
	// just-pressed and just-released state at the start of each frame.
	Manual bool
}

// Plugin samples raylib input into the Keys, MouseButtons and Mouse resources.
type Plugin struct{}

// Build implements [illusion.Plugin].
func (Plugin) Build(app *illusion.App) {
	app.InsertResource(
		illusion.R(NewButtonInput[Key]()),
		illusion.R(NewButtonInput[MouseButton]()),
		illusion.R(&Mouse{}),
	)
	app.InitResource(illusion.R(&Settings{}))
	app.AddSystems(illusion.PreUpdate,
		illusion.Fn2(sampleKeys).InSet(Sample).Named("input.keys"),
		illusion.Fn3(sampleMouse).InSet(Sample).Named("input.mouse"),
	)
}

func sampleKeys(res *illusion.Res[Keys], settings *illusion.Res[Settings]) {
	keys := res.Get()
	keys.Clear()
	if settings.Get().Manual {
		return
	}
	for k := range keys.pressed {
		if !rl.IsKeyDown(int32(k)) {
			keys.Release(k)
		}
	}
	for k := rl.GetKeyPressed(); k != 0; k = rl.GetKeyPressed() {
		keys.Press(Key(k))
	}
}

var mouseButtons = []MouseButton{
	rl.MouseButtonLeft, rl.MouseButtonRight, rl.MouseButtonMiddle,
	rl.MouseButtonSide, rl.MouseButtonExtra, rl.MouseButtonForward, rl.MouseButtonBack,
}

func sampleMouse(buttons *illusion.Res[MouseButtons], mouse *illusion.Res[Mouse], settings *illusion.Res[Settings]) {
	b := buttons.Get()
	b.Clear()
	if settings.Get().Manual {
		return
	}
	for _, btn := range mouseButtons {
		if rl.IsMouseButtonDown(btn) {
			b.Press(btn)
		} else {
			b.Release(btn)
		}
	}
	m := mouse.Get()
	m.Position = rl.GetMousePosition()
	m.Delta = rl.GetMouseDelta()
	m.Wheel = rl.GetMouseWheelMove()
}
