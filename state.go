package illusion

import (
	"fmt"

	"github.com/mlange-42/ark/ecs"
)

// State is a resource holding the current value of state type S. Register a
// state type with [AddState]; change it through [NextState].
type State[S comparable] struct {
	current S
}

// Get returns the current state.
func (s *State[S]) Get() S { return s.current }

// NextState is a resource for requesting a state change. The change happens
// in the next StateTransition schedule: OnExit of the old state runs, then
// OnEnter of the new one.
type NextState[S comparable] struct {
	next    S
	pending bool
}

// Set requests a change to s. Requesting the current state does nothing.
func (n *NextState[S]) Set(s S) {
	n.next, n.pending = s, true
}

// Pending returns the requested state, if there is one.
func (n *NextState[S]) Pending() (S, bool) {
	return n.next, n.pending
}

// DespawnOnExit despawns its entity (and the entity's children) when the app
// leaves State.
type DespawnOnExit[S comparable] struct {
	State S
}

type stateLabel[S comparable] struct {
	enter bool
	state S
}

func (l stateLabel[S]) String() string {
	if l.enter {
		return fmt.Sprintf("OnEnter(%v)", l.state)
	}
	return fmt.Sprintf("OnExit(%v)", l.state)
}

// OnEnter is the schedule that runs when the app enters state s.
func OnEnter[S comparable](s S) ScheduleLabel { return stateLabel[S]{enter: true, state: s} }

// OnExit is the schedule that runs when the app leaves state s.
func OnExit[S comparable](s S) ScheduleLabel { return stateLabel[S]{enter: false, state: s} }

// InState holds while state type S is s.
func InState[S comparable](s S) Condition {
	return Cond1(func(st *Res[State[S]]) bool {
		cur, ok := st.TryGet()
		return ok && cur.current == s
	})
}

// StateChanged holds on the frame state type S changed.
func StateChanged[S comparable]() Condition {
	return Cond2(func(st *Res[State[S]], last *Local[stateSeen[S]]) bool {
		cur, ok := st.TryGet()
		if !ok {
			return false
		}
		seen := last.Get()
		changed := !seen.valid || seen.state != cur.current
		seen.state, seen.valid = cur.current, true
		return changed
	})
}

type stateSeen[S comparable] struct {
	state S
	valid bool
}

// AddState registers state type S, starting at initial. OnEnter(initial) runs
// once, right after the startup schedules.
func AddState[S comparable](app *App, initial S) *App {
	if ecs.GetResource[State[S]](app.World) != nil {
		panic(fmt.Sprintf("illusion: the state type of %v was added twice", initial))
	}
	app.InsertResource(R(&State[S]{current: initial}), R(&NextState[S]{}))
	app.AddSystems(StateTransition,
		Sys(&stateTransition[S]{app: app}).Named(fmt.Sprintf("state transition (starting at %v)", initial)))
	return app
}

type stateTransition[S comparable] struct {
	app     *App
	state   *State[S]
	next    *NextState[S]
	scoped  *ecs.Filter1[DespawnOnExit[S]]
	entered bool
	doomed  []ecs.Entity
}

func (t *stateTransition[S]) Init(w *ecs.World) {
	t.state = ecs.GetResource[State[S]](w)
	t.next = ecs.GetResource[NextState[S]](w)
	t.scoped = ecs.NewFilter1[DespawnOnExit[S]](w).Register()
}

func (t *stateTransition[S]) Run(w *ecs.World) {
	if !t.entered {
		t.entered = true
		t.app.RunSchedule(OnEnter(t.state.current))
	}
	if !t.next.pending {
		return
	}
	next := t.next.next
	t.next.pending = false
	if next == t.state.current {
		return
	}

	prev := t.state.current
	t.app.RunSchedule(OnExit(prev))
	t.despawnScoped(w, prev)
	t.state.current = next
	t.app.RunSchedule(OnEnter(next))
}

func (t *stateTransition[S]) despawnScoped(w *ecs.World, s S) {
	t.doomed = t.doomed[:0]
	query := t.scoped.Query()
	for query.Next() {
		if query.Get().State == s {
			t.doomed = append(t.doomed, query.Entity())
		}
	}
	queue := ecs.GetResource[commandQueue](w)
	for _, e := range t.doomed {
		if w.Alive(e) { // may have gone with a despawned ancestor
			queue.despawn(w, e)
		}
	}
}
