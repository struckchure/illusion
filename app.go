package illusion

import (
	"time"

	"github.com/mlange-42/ark/ecs"
)

// Plugin configures an app: it adds systems, resources and other plugins.
//
// A plugin may also implement `Finish(*App)`, called once every plugin has been
// built, and `Cleanup(*App)`, called when the app shuts down.
type Plugin interface {
	Build(app *App)
}

// PluginFunc lets a plain function act as a plugin.
type PluginFunc func(app *App)

// Build implements [Plugin].
func (f PluginFunc) Build(app *App) { f(app) }

type pluginFinisher interface{ Finish(app *App) }
type pluginCleaner interface{ Cleanup(app *App) }

// Runner drives the app's main loop. It returns when the app should stop.
type Runner func(app *App)

// RunOnce is the default runner: it runs startup and a single frame.
func RunOnce(app *App) {
	app.Update()
}

// App holds the ECS world, schedules and plugins.
type App struct {
	World *ecs.World

	schedules map[ScheduleLabel]*Schedule
	created   []ScheduleLabel
	plugins   []Plugin
	runner    Runner
	commands  *commandQueue
	events    *eventRegistry
	time      *Time
	fixed     *FixedTime
	exit      *AppExit

	lastUpdate time.Time
	started    bool
	finished   bool
	cleanedUp  bool
}

// New creates an app with an empty world and the core resources: [Time],
// [FixedTime] and [AppExit].
func New() *App {
	app := &App{
		World:     ecs.NewWorld(),
		schedules: map[ScheduleLabel]*Schedule{},
		runner:    RunOnce,
		commands:  &commandQueue{},
		events:    &eventRegistry{},
		time:      &Time{},
		fixed:     &FixedTime{Timestep: time.Second / 60},
		exit:      &AppExit{},
	}
	ecs.AddResource(app.World, app.commands)
	ecs.AddResource(app.World, app.events)
	ecs.AddResource(app.World, app.time)
	ecs.AddResource(app.World, app.fixed)
	ecs.AddResource(app.World, app.exit)
	return app
}

// AddPlugins builds each plugin immediately, in order.
func (a *App) AddPlugins(plugins ...Plugin) *App {
	for _, p := range plugins {
		a.plugins = append(a.plugins, p)
		p.Build(a)
	}
	return a
}

// AddSystems adds systems to a schedule.
func (a *App) AddSystems(label ScheduleLabel, systems ...IntoSystems) *App {
	s := a.Schedule(label)
	for _, group := range systems {
		for _, c := range group.systemConfigs() {
			s.addSystem(c)
		}
	}
	return a
}

// ConfigureSets sets ordering and conditions for system sets in a schedule.
func (a *App) ConfigureSets(label ScheduleLabel, sets ...*SetConfig) *App {
	s := a.Schedule(label)
	for _, c := range sets {
		s.configureSet(c)
	}
	return a
}

// InsertResource adds resources, replacing any that already exist.
func (a *App) InsertResource(resources ...Resource) *App {
	for _, r := range resources {
		r.insert(a.World, true)
	}
	return a
}

// InitResource adds resources that don't exist yet and leaves existing ones
// alone. Plugins use it to provide defaults the user may have set already.
func (a *App) InitResource(resources ...Resource) *App {
	for _, r := range resources {
		r.insert(a.World, false)
	}
	return a
}

// SetRunner replaces the main loop. Backends like the raylib window plugin
// install their own.
func (a *App) SetRunner(r Runner) *App {
	a.runner = r
	return a
}

// Schedule returns the schedule for label, creating it if needed.
func (a *App) Schedule(label ScheduleLabel) *Schedule {
	s, ok := a.schedules[label]
	if !ok {
		s = newSchedule(label)
		a.schedules[label] = s
		a.created = append(a.created, label)
	}
	return s
}

// RunSchedule runs a schedule once, e.g. a custom one from an exclusive system.
func (a *App) RunSchedule(label ScheduleLabel) {
	if s, ok := a.schedules[label]; ok {
		s.run(a.World, a.flush)
	}
}

// Run finishes plugin setup, hands control to the runner, then cleans up.
func (a *App) Run() {
	a.finish()
	defer a.Cleanup()
	a.runner(a)
}

// ShouldExit reports whether something asked the app to stop.
func (a *App) ShouldExit() bool {
	return a.exit.Requested
}

// Startup runs the startup schedules. It does nothing after the first call,
// and Update calls it automatically.
func (a *App) Startup() {
	if a.started {
		return
	}
	a.finish()
	a.started = true
	for _, label := range startupSchedules {
		a.RunSchedule(label)
	}
	a.lastUpdate = time.Now()
}

// Update runs one frame, using the wall clock for the delta.
func (a *App) Update() {
	a.Startup()
	now := time.Now()
	dt := now.Sub(a.lastUpdate)
	a.lastUpdate = now
	a.Tick(dt)
}

// Tick runs one frame with an explicit delta. Use it for tests and replays.
func (a *App) Tick(dt time.Duration) {
	a.Startup()
	dt = min(max(dt, 0), maxDelta)

	a.time.delta = dt
	a.time.elapsed += dt
	a.events.update()
	for _, label := range preFixedSchedules {
		a.RunSchedule(label)
	}
	a.runFixed(dt)
	for _, label := range postFixedSchedules {
		a.RunSchedule(label)
	}
	a.time.frame++
}

// runFixed runs the Fixed* schedules once per whole timestep accumulated,
// with Time reporting the fixed step while they run.
func (a *App) runFixed(dt time.Duration) {
	f := a.fixed
	if f.Timestep <= 0 {
		return
	}
	f.accumulated += dt
	if f.accumulated < f.Timestep {
		return
	}

	frameDelta, frameElapsed := a.time.delta, a.time.elapsed
	a.time.fixed = true
	for f.accumulated >= f.Timestep {
		f.accumulated -= f.Timestep
		f.elapsed += f.Timestep
		a.time.delta, a.time.elapsed = f.Timestep, f.elapsed
		for _, label := range fixedSchedules {
			a.RunSchedule(label)
		}
	}
	a.time.delta, a.time.elapsed = frameDelta, frameElapsed
	a.time.fixed = false
}

// Cleanup calls Cleanup on systems and plugins that implement it, in reverse
// order. Run calls it automatically; runners that own resources which must
// outlive cleanup (like a graphics context) call it themselves first. Only the
// first call does anything.
func (a *App) Cleanup() {
	if a.cleanedUp {
		return
	}
	a.cleanedUp = true
	for i := len(a.created) - 1; i >= 0; i-- {
		a.schedules[a.created[i]].cleanup(a.World)
	}
	for i := len(a.plugins) - 1; i >= 0; i-- {
		if p, ok := a.plugins[i].(pluginCleaner); ok {
			p.Cleanup(a)
		}
	}
}

func (a *App) finish() {
	if a.finished {
		return
	}
	a.finished = true
	for _, p := range a.plugins {
		if f, ok := p.(pluginFinisher); ok {
			f.Finish(a)
		}
	}
}

func (a *App) flush() {
	if len(a.commands.commands) > 0 {
		a.commands.apply(a.World)
	}
}
