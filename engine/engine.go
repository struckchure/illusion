// Package engine bundles the standard plugins.
package engine

import (
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
	"github.com/struckchure/illusion/audio"
	"github.com/struckchure/illusion/diag"
	"github.com/struckchure/illusion/input"
	"github.com/struckchure/illusion/render"
	"github.com/struckchure/illusion/transform"
	"github.com/struckchure/illusion/window"
)

// Config configures the default plugins.
type Config struct {
	Window window.Config
	// AssetRoot is the directory asset paths are relative to. Defaults to
	// "assets".
	AssetRoot string
}

// DefaultPlugins opens a window and adds assets, input, transforms,
// rendering, audio and diagnostics (gizmos, and a stats overlay toggled with
// F3). Physics is separate: add physics.Plugin.
func DefaultPlugins(cfg Config) illusion.Plugin {
	return illusion.PluginFunc(func(app *illusion.App) {
		app.AddPlugins(
			window.Plugin{Config: cfg.Window},
			asset.Plugin{Root: cfg.AssetRoot},
			input.Plugin{},
			transform.Plugin{},
			render.Plugin{},
			audio.Plugin{},
			diag.Plugin{},
		)
	})
}
