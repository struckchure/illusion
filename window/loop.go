//go:build !js

package window

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/struckchure/illusion"
)

func run(app *illusion.App, cfg Config) {
	open(cfg)
	defer rl.CloseWindow()
	// Release GPU resources while the context is still alive.
	defer app.Cleanup()

	for !app.ShouldExit() {
		app.Update()
	}
}
