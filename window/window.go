// Package window opens a raylib window and drives the app's main loop.
package window

import (
	"runtime"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/struckchure/illusion"
)

func init() {
	// raylib (OpenGL and the OS windowing APIs) only works from the main
	// thread. init runs on it, so pin the main goroutine there for good.
	runtime.LockOSThread()
}

// Config describes the window. Zero fields get sensible defaults.
type Config struct {
	// Title defaults to "Illusion".
	Title string
	// Width and Height default to 1280x720.
	Width, Height int
	// TargetFPS defaults to 60. Negative means unlimited.
	TargetFPS int
	// VSync syncs presentation with the display.
	VSync bool
	// Resizable lets the user resize the window.
	Resizable bool
	// MSAA enables 4x multisample anti-aliasing.
	MSAA bool
	// KeepEscape stops Escape from closing the window (raylib's default).
	KeepEscape bool
}

func (c Config) withDefaults() Config {
	if c.Title == "" {
		c.Title = "Illusion"
	}
	if c.Width <= 0 {
		c.Width = 1280
	}
	if c.Height <= 0 {
		c.Height = 720
	}
	if c.TargetFPS == 0 {
		c.TargetFPS = 60
	}
	return c
}

// Window is a resource describing the open window. It is refreshed at the
// start of every frame.
type Window struct {
	// Width and Height are the current size in screen coordinates.
	Width, Height int
	// Resized is true on frames where the size changed.
	Resized bool
	// Focused is true while the window has keyboard focus.
	Focused bool

	title string
}

// Title returns the window title.
func (w *Window) Title() string { return w.title }

// SetTitle changes the window title.
func (w *Window) SetTitle(title string) {
	w.title = title
	rl.SetWindowTitle(title)
}

// Plugin opens the window and installs a raylib main loop as the app runner.
type Plugin struct {
	Config Config
}

// Build implements [illusion.Plugin].
func (p Plugin) Build(app *illusion.App) {
	cfg := p.Config.withDefaults()
	app.InsertResource(illusion.R(&Window{Width: cfg.Width, Height: cfg.Height, title: cfg.Title}))
	app.AddSystems(illusion.First, illusion.Fn2(updateWindow).Named("window.update"))
	app.SetRunner(func(app *illusion.App) { run(app, cfg) })
}

// open creates the window and applies cfg. The main loop that follows is
// platform specific: see loop.go and loop_js.go.
func open(cfg Config) {
	var flags uint32
	if cfg.VSync {
		flags |= rl.FlagVsyncHint
	}
	if cfg.Resizable {
		flags |= rl.FlagWindowResizable
	}
	if cfg.MSAA {
		flags |= rl.FlagMsaa4xHint
	}
	rl.SetConfigFlags(flags)
	rl.SetTraceLogLevel(rl.LogWarning)
	rl.InitWindow(int32(cfg.Width), int32(cfg.Height), cfg.Title)

	if cfg.KeepEscape {
		rl.SetExitKey(rl.KeyNull)
	}
	if cfg.TargetFPS > 0 {
		rl.SetTargetFPS(int32(cfg.TargetFPS))
	}
}

func updateWindow(win *illusion.Res[Window], exit *illusion.Res[illusion.AppExit]) {
	w := win.Get()
	width, height := rl.GetScreenWidth(), rl.GetScreenHeight()
	w.Resized = width != w.Width || height != w.Height
	w.Width, w.Height = width, height
	w.Focused = rl.IsWindowFocused()
	if rl.WindowShouldClose() {
		exit.Get().Requested = true
	}
}
