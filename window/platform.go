//go:build !js

package window

import rl "github.com/gen2brain/raylib-go/raylib"

// platformFlags are the raylib flags for cfg's platform-dependent settings.
// On the desktop, raylib handles both.
func platformFlags(cfg Config) uint32 {
	var flags uint32
	if cfg.Resizable {
		flags |= rl.FlagWindowResizable
	}
	if cfg.HighDPI {
		flags |= rl.FlagWindowHighdpi
	}
	return flags
}

// initialSize is the size to open the window at.
func initialSize(cfg Config) (width, height int) { return cfg.Width, cfg.Height }

// fit keeps the window matched to its surroundings. The desktop's window
// manager does that itself.
func fit(Config) {}

// scale is Window.Scale. With HighDPI, raylib scales 2D drawing itself.
func scale(Config) float32 { return 1 }
