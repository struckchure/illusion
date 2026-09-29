package illusion

import (
	"slices"
	"testing"

	"github.com/mlange-42/ark/ecs"
)

type gameState int

const (
	menu gameState = iota
	playing
	paused
)

func (s gameState) String() string { return [...]string{"menu", "playing", "paused"}[s] }

func TestStates(t *testing.T) {
	var log []string
	add := func(msg string) *SystemConfig { return Fn0(func() { log = append(log, msg) }) }

	app := New()
	AddState(app, menu)
	app.
		AddSystems(Startup, add("startup")).
		AddSystems(OnEnter(menu), add("enter menu")).
		AddSystems(OnExit(menu), add("exit menu")).
		AddSystems(OnEnter(playing), add("enter playing"), Fn1(func(cmd *Commands) {
			cmd.Spawn(C(score{}), C(DespawnOnExit[gameState]{State: playing})).
				WithChild(C(name("child")))
		})).
		AddSystems(OnExit(playing), add("exit playing")).
		AddSystems(Update,
			add("menu tick").RunIf(InState(menu)),
			add("playing tick").RunIf(InState(playing)),
			add("changed").RunIf(StateChanged[gameState]()),
		)

	next := ecs.GetResource[NextState[gameState]](app.World)
	app.Tick(0)
	next.Set(playing)
	app.Tick(0)
	next.Set(playing) // same state: no transition
	app.Tick(0)

	var q Query0Where[NoFilter]
	q.InitParam(app.World)
	if q.Count() != 2 {
		t.Fatalf("expected the scoped entity and its child, got %d entities", q.Count())
	}

	next.Set(paused)
	app.Tick(0)
	if q.Count() != 0 {
		t.Fatalf("leaving playing should despawn scoped entities, %d left", q.Count())
	}

	want := []string{
		"startup", "enter menu",
		"menu tick", "changed",
		"exit menu", "enter playing", "playing tick", "changed",
		"playing tick",
		"exit playing", "changed",
	}
	if !slices.Equal(log, want) {
		t.Fatalf("got  %q\nwant %q", log, want)
	}
	if s := ecs.GetResource[State[gameState]](app.World).Get(); s != paused {
		t.Fatalf("state %v", s)
	}
	if got := OnEnter(paused).String(); got != "OnEnter(paused)" {
		t.Fatalf("label %q", got)
	}
}

func TestStateLabelsDontCollideAcrossTypes(t *testing.T) {
	type other int
	app := New()
	AddState(app, menu)
	AddState(app, other(0))
	var log []string
	app.AddSystems(OnEnter(menu), Fn0(func() { log = append(log, "game") })).
		AddSystems(OnEnter(other(0)), Fn0(func() { log = append(log, "other") }))
	app.Tick(0)
	if !slices.Equal(log, []string{"game", "other"}) {
		t.Fatalf("got %v", log)
	}
}
