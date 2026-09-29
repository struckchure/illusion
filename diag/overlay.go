package diag

import (
	"fmt"
	"slices"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/input"
	"github.com/struckchure/illusion/render"
)

// Overlay is a resource controlling the stats overlay.
type Overlay struct {
	// Visible shows the overlay.
	Visible bool
}

// Stats is a resource of extra lines for the overlay. Plugins and games set
// named values each frame (e.g. "bodies": "42").
type Stats struct {
	values map[string]string
}

// Set shows name: value on the overlay.
func (s *Stats) Set(name, value string) {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[name] = value
}

// Get returns a stat's value, or "" if it isn't set.
func (s *Stats) Get(name string) string { return s.values[name] }

// Remove hides a stat.
func (s *Stats) Remove(name string) { delete(s.values, name) }

// Plugin adds gizmos and the stats overlay.
type Plugin struct {
	// ShowOverlay starts with the overlay visible.
	ShowOverlay bool
	// ToggleKey shows and hides the overlay; 0 means F3.
	ToggleKey int32
}

// Build implements [illusion.Plugin].
func (p Plugin) Build(app *illusion.App) {
	key := p.ToggleKey
	if key == 0 {
		key = rl.KeyF3
	}
	app.InitResource(illusion.R(&Overlay{Visible: p.ShowOverlay}), illusion.R(&Stats{}))
	buildGizmos(app)
	app.AddSystems(illusion.Update, illusion.Fn2(func(keys *illusion.Res[input.Keys], o *illusion.Res[Overlay]) {
		if keys.Get().JustPressed(input.Key(key)) {
			o.Get().Visible = !o.Get().Visible
		}
	}).RunIf(illusion.ResourceExists[input.Keys]()).Named("diag.toggleOverlay"))
	app.AddSystems(illusion.Render, illusion.Fn5(drawOverlay).
		InSet(render.Draw2D).
		RunIf(
			illusion.ResourceExists[render.View3D](), // only with a window to draw in
			illusion.Cond1(func(o *illusion.Res[Overlay]) bool { return o.Get().Visible }),
		).
		Named("diag.overlay"))
}

// frameTimes keeps a smoothed frame time and reusable buffers for the overlay.
type frameTimes struct {
	smoothed float32
	lines    []string
	names    []string
}

func drawOverlay(
	t *illusion.Res[illusion.Time],
	entities *illusion.Query0Where[illusion.NoFilter],
	stats *illusion.Res[Stats],
	gizmos *illusion.Res[gizmoBuffer],
	ft *illusion.Local[frameTimes],
) {
	f := ft.Get()
	dt := t.Get().DeltaSecs() * 1000
	if f.smoothed == 0 {
		f.smoothed = dt
	}
	f.smoothed += (dt - f.smoothed) * 0.05

	f.lines = append(f.lines[:0],
		fmt.Sprintf("fps       %d", rl.GetFPS()),
		fmt.Sprintf("frame     %.2f ms", f.smoothed),
		fmt.Sprintf("entities  %d", entities.Count()),
		fmt.Sprintf("gizmos    %d", gizmos.Get().count()),
	)
	f.names = f.names[:0]
	for name := range stats.Get().values {
		f.names = append(f.names, name)
	}
	slices.Sort(f.names)
	for _, name := range f.names {
		f.lines = append(f.lines, fmt.Sprintf("%-9s %s", name, stats.Get().values[name]))
	}

	const size, pad, lineHeight = 18, 10, 22
	width := int32(0)
	for _, l := range f.lines {
		width = max(width, rl.MeasureText(l, size))
	}
	x := int32(rl.GetScreenWidth()) - width - 2*pad - 10
	rl.DrawRectangle(x, 10, width+2*pad, int32(len(f.lines))*lineHeight+2*pad-4, rl.Fade(rl.Black, 0.6))
	for i, l := range f.lines {
		rl.DrawText(l, x+pad, 10+pad+int32(i)*lineHeight, size, rl.RayWhite)
	}
}
