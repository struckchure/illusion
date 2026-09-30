//go:build js

package window

import (
	"fmt"
	"math"
	"syscall/js"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// In a browser, illusion sizes the canvas instead of raylib, whose resize
// handler ignores devicePixelRatio. The canvas's CSS size is the page's
// (Resizable) or Width x Height, and its drawing buffer is that times
// devicePixelRatio with HighDPI. fit checks both every frame, so the canvas
// follows the page, zooming and moves between screens.

func platformFlags(Config) uint32 { return 0 }

func initialSize(cfg Config) (width, height int) { return bufferSize(cfg) }

// cssSize is the canvas's size on the page, in CSS pixels.
func cssSize(cfg Config) (width, height float64) {
	if cfg.Resizable {
		g := js.Global()
		return g.Get("innerWidth").Float(), g.Get("innerHeight").Float()
	}
	return float64(cfg.Width), float64(cfg.Height)
}

func pixelRatio(cfg Config) float64 {
	if !cfg.HighDPI {
		return 1
	}
	if r := js.Global().Get("devicePixelRatio"); r.Type() == js.TypeNumber && r.Float() > 0 {
		return r.Float()
	}
	return 1
}

// bufferSize is the canvas's drawing buffer size, in device pixels.
func bufferSize(cfg Config) (width, height int) {
	w, h := cssSize(cfg)
	r := pixelRatio(cfg)
	return max(1, int(math.Round(w*r))), max(1, int(math.Round(h*r)))
}

func scale(cfg Config) float32 { return float32(pixelRatio(cfg)) }

// canvasStyle holds the rule that sets the canvas's CSS size. It has to be a
// stylesheet rule: emscripten clears the canvas's inline size whenever it
// resizes the drawing buffer.
var (
	canvasStyle js.Value
	canvasRule  string
)

// cssRule sizes the canvas on the page. A fixed-size canvas at ratio 1
// needs none: the page's own styles apply, and shrink it to fit small pages.
func cssRule(cfg Config) string {
	w, h := cssSize(cfg)
	switch {
	case cfg.Resizable:
		return fmt.Sprintf(".illusion-canvas { width: %gpx !important; height: %gpx !important; }", w, h)
	case pixelRatio(cfg) != 1:
		// height: auto keeps the aspect ratio when max-width shrinks it.
		return fmt.Sprintf(".illusion-canvas { width: %gpx !important; height: auto !important; }", w)
	}
	return ""
}

func fit(cfg Config) {
	if w, h := bufferSize(cfg); w != rl.GetScreenWidth() || h != rl.GetScreenHeight() {
		rl.SetWindowSize(w, h)
	}
	rule := cssRule(cfg)
	if rule == canvasRule {
		return
	}
	doc := js.Global().Get("document")
	if canvasStyle.IsUndefined() {
		canvas := js.Global().Get("raylib").Get("canvas")
		if canvas.IsUndefined() {
			return
		}
		canvas.Get("classList").Call("add", "illusion-canvas")
		canvasStyle = doc.Call("createElement", "style")
		doc.Get("head").Call("append", canvasStyle)
	}
	canvasStyle.Set("textContent", rule)
	canvasRule = rule
}
