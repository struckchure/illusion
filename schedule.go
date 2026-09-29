package illusion

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mlange-42/ark/ecs"
)

// ScheduleLabel identifies a schedule. Labels are compared with ==, so
// implementations must be comparable. Most are a [ScheduleName]; state
// schedules come from [OnEnter] and [OnExit].
type ScheduleLabel interface {
	String() string
}

// ScheduleName is a schedule label that is just a name. Use it for custom
// schedules you run with [App.RunSchedule].
type ScheduleName string

func (n ScheduleName) String() string { return string(n) }

// Startup schedules run once, in this order, before the first frame. The
// StateTransition schedule then runs once to enter each state's initial value.
const (
	PreStartup  ScheduleName = "PreStartup"
	Startup     ScheduleName = "Startup"
	PostStartup ScheduleName = "PostStartup"
)

// Frame schedules run every frame, in this order. The Fixed* schedules run
// zero or more times per frame, at the rate set by [FixedTime].
const (
	First           ScheduleName = "First"
	PreUpdate       ScheduleName = "PreUpdate"
	StateTransition ScheduleName = "StateTransition"
	FixedFirst      ScheduleName = "FixedFirst"
	FixedPreUpdate  ScheduleName = "FixedPreUpdate"
	FixedUpdate     ScheduleName = "FixedUpdate"
	FixedPostUpdate ScheduleName = "FixedPostUpdate"
	FixedLast       ScheduleName = "FixedLast"
	Update          ScheduleName = "Update"
	PostUpdate      ScheduleName = "PostUpdate"
	Render          ScheduleName = "Render"
	Last            ScheduleName = "Last"
)

var (
	startupSchedules   = []ScheduleLabel{PreStartup, Startup, PostStartup, StateTransition}
	preFixedSchedules  = []ScheduleLabel{First, PreUpdate, StateTransition}
	fixedSchedules     = []ScheduleLabel{FixedFirst, FixedPreUpdate, FixedUpdate, FixedPostUpdate, FixedLast}
	postFixedSchedules = []ScheduleLabel{Update, PostUpdate, Render, Last}
)

// Schedule is an ordered collection of systems.
type Schedule struct {
	label      ScheduleLabel
	systems    []*SystemConfig
	sets       []*SetConfig
	order      []scheduledSystem
	gates      [][]Condition // conditions of each gated set
	gateResult []int8        // per run: 0 unchecked, 1 open, 2 closed
	inited     map[System]bool
	initedCond map[Condition]bool
	dirty      bool
}

type scheduledSystem struct {
	cfg        *SystemConfig
	conditions []Condition // the system's own
	gates      []int       // indexes into Schedule.gates for its gated sets
}

func newSchedule(label ScheduleLabel) *Schedule {
	return &Schedule{
		label:      label,
		inited:     map[System]bool{},
		initedCond: map[Condition]bool{},
	}
}

// Label returns the schedule's label.
func (s *Schedule) Label() ScheduleLabel { return s.label }

func (s *Schedule) addSystem(c *SystemConfig) {
	if c.added {
		panic(fmt.Sprintf("illusion: system %q was added twice", c.name))
	}
	c.added = true
	s.systems = append(s.systems, c)
	s.dirty = true
}

func (s *Schedule) configureSet(c *SetConfig) {
	s.sets = append(s.sets, c)
	s.dirty = true
}

// run executes every system whose conditions hold. flush is called after each
// system so its commands take effect before the next one runs.
func (s *Schedule) run(w *ecs.World, flush func()) {
	if s.dirty {
		s.build(w)
	}
	clear(s.gateResult)
	for _, sys := range s.order {
		if !s.gatesOpen(w, sys.gates) || !checkAll(w, sys.conditions) {
			continue
		}
		sys.cfg.system.Run(w)
		flush()
	}
}

// gatesOpen checks a system's set conditions. Each set's conditions are
// checked once per run, when its first system comes up, and the result is
// shared by the rest of the set; that keeps stateful conditions like Once
// consistent across the set.
func (s *Schedule) gatesOpen(w *ecs.World, gates []int) bool {
	for _, g := range gates {
		if s.gateResult[g] == 0 {
			s.gateResult[g] = 2
			if checkAll(w, s.gates[g]) {
				s.gateResult[g] = 1
			}
		}
		if s.gateResult[g] == 2 {
			return false
		}
	}
	return true
}

func checkAll(w *ecs.World, conds []Condition) bool {
	for _, c := range conds {
		if !c.Check(w) {
			return false
		}
	}
	return true
}

// build sorts the systems by their ordering constraints and initializes any
// new systems and conditions. Ties keep insertion order.
func (s *Schedule) build(w *ecs.World) {
	n := len(s.systems)
	index := make(map[*SystemConfig]int, n)
	members := map[SystemSet][]int{}
	for i, c := range s.systems {
		index[c] = i
		for _, set := range c.sets {
			members[set] = append(members[set], i)
		}
	}

	resolve := func(owner string, l Label) []int {
		switch l := l.(type) {
		case *SystemConfig:
			i, ok := index[l]
			if !ok {
				panic(fmt.Sprintf("illusion: %s is ordered against system %q, which is not in schedule %s", owner, l.name, s.label))
			}
			return []int{i}
		case SystemSet:
			return members[l]
		}
		return nil
	}

	succ := make([][]int, n)
	indeg := make([]int, n)
	edge := func(from, to int) {
		succ[from] = append(succ[from], to)
		indeg[to]++
	}
	constrain := func(owner string, targets []int, before, after []Label) {
		for _, l := range before {
			for _, j := range resolve(owner, l) {
				for _, i := range targets {
					edge(i, j)
				}
			}
		}
		for _, l := range after {
			for _, j := range resolve(owner, l) {
				for _, i := range targets {
					edge(j, i)
				}
			}
		}
	}

	for i, c := range s.systems {
		constrain(fmt.Sprintf("system %q", c.name), []int{i}, c.before, c.after)
	}
	gateOf := map[SystemSet]int{}
	var gates [][]Condition
	for _, sc := range s.sets {
		constrain(fmt.Sprintf("set %q", sc.set), members[sc.set], sc.before, sc.after)
		if len(sc.conditions) == 0 {
			continue
		}
		g, ok := gateOf[sc.set]
		if !ok {
			g = len(gates)
			gateOf[sc.set] = g
			gates = append(gates, nil)
		}
		gates[g] = append(gates[g], sc.conditions...)
	}

	// Kahn's algorithm, always picking the earliest-added ready system.
	done := make([]bool, n)
	order := make([]scheduledSystem, 0, n)
	for len(order) < n {
		next := -1
		for i := 0; i < n; i++ {
			if !done[i] && indeg[i] == 0 {
				next = i
				break
			}
		}
		if next < 0 {
			panic(s.cycleError(done))
		}
		done[next] = true
		for _, j := range succ[next] {
			indeg[j]--
		}
		c := s.systems[next]
		sys := scheduledSystem{cfg: c, conditions: c.conditions}
		for _, set := range c.sets {
			if g, ok := gateOf[set]; ok && !slices.Contains(sys.gates, g) {
				sys.gates = append(sys.gates, g)
			}
		}
		order = append(order, sys)
	}

	initCond := func(c Condition) {
		if !s.initedCond[c] {
			s.initedCond[c] = true
			c.Init(w)
		}
	}
	for _, sys := range order {
		if !s.inited[sys.cfg.system] {
			s.inited[sys.cfg.system] = true
			sys.cfg.system.Init(w)
		}
		for _, c := range sys.conditions {
			initCond(c)
		}
	}
	for _, conds := range gates {
		for _, c := range conds {
			initCond(c)
		}
	}

	s.order = order
	s.gates = gates
	s.gateResult = make([]int8, len(gates))
	s.dirty = false
}

func (s *Schedule) cycleError(done []bool) string {
	names := []string{}
	for i, c := range s.systems {
		if !done[i] {
			names = append(names, fmt.Sprintf("%q", c.name))
		}
	}
	return fmt.Sprintf("illusion: ordering cycle in schedule %s between systems %s", s.label, strings.Join(names, ", "))
}

func (s *Schedule) cleanup(w *ecs.World) {
	for i := len(s.order) - 1; i >= 0; i-- {
		if c, ok := s.order[i].cfg.system.(SystemCleanup); ok {
			c.Cleanup(w)
		}
	}
}
