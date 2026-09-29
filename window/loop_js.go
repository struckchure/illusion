//go:build js

package window

import (
	"syscall/js"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/struckchure/illusion"
)

// run drives the app from requestAnimationFrame: a browser tab can't block in
// a loop, so each frame is a callback. run returns once something requests
// AppExit (closing the tab just stops everything).
func run(app *illusion.App, cfg Config) {
	open(cfg)

	done := make(chan struct{})
	var frame js.Func
	frame = js.FuncOf(func(js.Value, []js.Value) any {
		if app.ShouldExit() {
			frame.Release()
			close(done)
			return nil
		}
		app.Update()
		js.Global().Call("requestAnimationFrame", frame)
		return nil
	})
	js.Global().Call("requestAnimationFrame", frame)
	<-done

	// Release GPU resources while the context is still alive.
	app.Cleanup()
	rl.CloseWindow()
}
