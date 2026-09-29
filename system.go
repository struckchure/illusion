package illusion

import (
	"runtime"
	"strings"
	"unsafe"

	"github.com/mlange-42/ark/ecs"
)

//go:generate go run ./internal/gen

// System is the unit a schedule runs.
//
// Most systems are plain functions wrapped with Fn0..Fn8. Implement System
// directly when a system needs setup that parameters can't express; wrap it
// with [Sys] to add it to an app. Params can still be used from a struct
// system by calling their InitParam from Init.
type System interface {
	// Init runs once, before the system's first run.
	Init(w *ecs.World)
	// Run runs every time the system's schedule runs.
	Run(w *ecs.World)
}

// SystemCleanup is implemented by systems that release resources when the
// app shuts down.
type SystemCleanup interface {
	Cleanup(w *ecs.World)
}

// Condition decides whether a system (or every system in a set) runs.
// Build one from a function with Cond0..Cond8.
type Condition interface {
	Init(w *ecs.World)
	Check(w *ecs.World) bool
}

// Label names something systems can be ordered against: a single system
// (a *SystemConfig) or a [SystemSet].
type Label interface {
	isLabel()
}

// SystemSet groups systems so they can be ordered and gated together.
type SystemSet string

func (SystemSet) isLabel() {}

// Before configures every system in s to run before the given labels.
func (s SystemSet) Before(labels ...Label) *SetConfig {
	return (&SetConfig{set: s}).Before(labels...)
}

// After configures every system in s to run after the given labels.
func (s SystemSet) After(labels ...Label) *SetConfig {
	return (&SetConfig{set: s}).After(labels...)
}

// RunIf gates every system in s on the given conditions.
func (s SystemSet) RunIf(conds ...Condition) *SetConfig {
	return (&SetConfig{set: s}).RunIf(conds...)
}

// SetConfig holds ordering and conditions shared by every system in a set.
// Apply it with [App.ConfigureSets].
type SetConfig struct {
	set        SystemSet
	before     []Label
	after      []Label
	conditions []Condition
}

// Before adds labels the set must run before.
func (c *SetConfig) Before(labels ...Label) *SetConfig {
	c.before = append(c.before, labels...)
	return c
}

// After adds labels the set must run after.
func (c *SetConfig) After(labels ...Label) *SetConfig {
	c.after = append(c.after, labels...)
	return c
}

// RunIf adds conditions that all must hold for the set's systems to run.
func (c *SetConfig) RunIf(conds ...Condition) *SetConfig {
	c.conditions = append(c.conditions, conds...)
	return c
}

// SystemConfig is a system plus its scheduling metadata. The pointer itself
// identifies the system, so keep it around to order other systems against it:
//
//	move := illusion.Fn2(movePlayer)
//	app.AddSystems(illusion.Update, move, illusion.Fn1(followCamera).After(move))
type SystemConfig struct {
	name       string
	system     System
	sets       []SystemSet
	before     []Label
	after      []Label
	conditions []Condition
	added      bool
}

func (*SystemConfig) isLabel() {}

func newSystemConfig(name string, s System) *SystemConfig {
	return &SystemConfig{name: name, system: s}
}

// Sys wraps a struct system so it can be added to an app. If s has a
// `Name() string` method it names the system; otherwise use [SystemConfig.Named].
func Sys(s System) *SystemConfig {
	name := "system"
	if n, ok := s.(interface{ Name() string }); ok {
		name = n.Name()
	}
	return newSystemConfig(name, s)
}

// Name returns the system's name, used in error messages.
func (c *SystemConfig) Name() string { return c.name }

// Named overrides the system's name.
func (c *SystemConfig) Named(name string) *SystemConfig {
	c.name = name
	return c
}

// Before orders this system before the given systems or sets.
func (c *SystemConfig) Before(labels ...Label) *SystemConfig {
	c.before = append(c.before, labels...)
	return c
}

// After orders this system after the given systems or sets.
func (c *SystemConfig) After(labels ...Label) *SystemConfig {
	c.after = append(c.after, labels...)
	return c
}

// InSet adds this system to sets.
func (c *SystemConfig) InSet(sets ...SystemSet) *SystemConfig {
	c.sets = append(c.sets, sets...)
	return c
}

// RunIf adds conditions that all must hold for this system to run.
func (c *SystemConfig) RunIf(conds ...Condition) *SystemConfig {
	c.conditions = append(c.conditions, conds...)
	return c
}

func (c *SystemConfig) systemConfigs() []*SystemConfig { return []*SystemConfig{c} }

// IntoSystems is anything that can be passed to [App.AddSystems]: a single
// *SystemConfig or a *SystemGroup.
type IntoSystems interface {
	systemConfigs() []*SystemConfig
}

// SystemGroup configures several systems at once. Build one with [Group] or
// [Chain]; its methods apply to every system in it.
type SystemGroup struct {
	systems []*SystemConfig
}

// Group bundles systems so they can be configured together.
func Group(systems ...*SystemConfig) *SystemGroup {
	return &SystemGroup{systems: systems}
}

// Chain bundles systems and runs them in order, each after the one before.
func Chain(systems ...*SystemConfig) *SystemGroup {
	for i := 1; i < len(systems); i++ {
		systems[i].After(systems[i-1])
	}
	return Group(systems...)
}

// InSet adds every system in the group to sets.
func (g *SystemGroup) InSet(sets ...SystemSet) *SystemGroup {
	for _, s := range g.systems {
		s.InSet(sets...)
	}
	return g
}

// RunIf gates every system in the group on conds.
func (g *SystemGroup) RunIf(conds ...Condition) *SystemGroup {
	for _, s := range g.systems {
		s.RunIf(conds...)
	}
	return g
}

// Before orders every system in the group before labels.
func (g *SystemGroup) Before(labels ...Label) *SystemGroup {
	for _, s := range g.systems {
		s.Before(labels...)
	}
	return g
}

// After orders every system in the group after labels.
func (g *SystemGroup) After(labels ...Label) *SystemGroup {
	for _, s := range g.systems {
		s.After(labels...)
	}
	return g
}

func (g *SystemGroup) systemConfigs() []*SystemConfig { return g.systems }

// funcName returns the name of the function f for error messages. It reads the
// code pointer from the func value directly to avoid reflection.
func funcName[F any](f F) string {
	pc := **(**uintptr)(unsafe.Pointer(&f))
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "system"
	}
	name := fn.Name()
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return name
}
