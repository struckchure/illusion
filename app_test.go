package illusion

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mlange-42/ark/ecs"
)

type position struct{ X, Y float32 }
type velocity struct{ X, Y float32 }
type player struct{}
type dead struct{}
type score struct{ Value int }

func move(q *Query2[position, velocity], t *Res[Time]) {
	dt := t.Get().DeltaSecs()
	q.Each(func(_ ecs.Entity, p *position, v *velocity) {
		p.X += v.X * dt
		p.Y += v.Y * dt
	})
}

func TestFunctionSystems(t *testing.T) {
	app := New().
		AddSystems(Startup, Fn1(func(cmd *Commands) {
			cmd.Spawn(C(position{}), C(velocity{X: 1, Y: 2}))
			cmd.Spawn(C(position{}))
		})).
		AddSystems(Update, Fn2(move))

	app.Tick(125 * time.Millisecond)
	app.Tick(time.Second) // clamped to 250ms

	var q Query2[position, velocity]
	q.InitParam(app.World)
	if n := q.Count(); n != 1 {
		t.Fatalf("expected 1 moving entity, got %d", n)
	}
	_, p, _, ok := q.Single()
	if !ok || p.X != 0.375 || p.Y != 0.75 {
		t.Fatalf("unexpected position %+v", p)
	}
}

func TestLocalPersists(t *testing.T) {
	var seen []int
	app := New().AddSystems(Update, Fn1(func(n *Local[int]) {
		*n.Get()++
		seen = append(seen, *n.Get())
	}))
	for range 3 {
		app.Tick(0)
	}
	if !slices.Equal(seen, []int{1, 2, 3}) {
		t.Fatalf("got %v", seen)
	}
}

func TestCommandsInsideQuery(t *testing.T) {
	app := New().
		AddSystems(Startup, Fn1(func(cmd *Commands) {
			for i := range 5 {
				cmd.Spawn(C(score{Value: i}))
			}
		})).
		AddSystems(Update, Fn2(func(q *Query1[score], cmd *Commands) {
			query := q.Iter()
			for query.Next() {
				if query.Get().Value%2 == 0 {
					cmd.Despawn(query.Entity())
				} else {
					cmd.Entity(query.Entity()).Insert(C(dead{}))
				}
			}
		}))
	app.Tick(0)

	var alive Query1Where[score, Without[dead]]
	alive.InitParam(app.World)
	var gone Query1Where[score, With[dead]]
	gone.InitParam(app.World)
	if alive.Count() != 0 || gone.Count() != 2 {
		t.Fatalf("alive=%d dead=%d", alive.Count(), gone.Count())
	}
}

func TestEntityCommands(t *testing.T) {
	var spawned ecs.Entity
	app := New().AddSystems(Startup, Fn1(func(cmd *Commands) {
		cmd.Spawn(C(score{Value: 1}), C(velocity{})).
			Insert(C(score{Value: 7}), C(player{})).
			Remove(ecs.C[velocity]()).
			Then(func(_ *ecs.World, e ecs.Entity) { spawned = e })
	}))
	app.Tick(0)

	var q Query1Where[score, And[With[player], Without[velocity]]]
	q.InitParam(app.World)
	s, ok := q.Get(spawned)
	if !ok || s.Value != 7 {
		t.Fatalf("expected score 7 on %v, got %+v ok=%v", spawned, s, ok)
	}
	if !q.Contains(spawned) {
		t.Fatal("entity should match the filter")
	}
}

func TestSpawnDuplicateComponentKeepsLast(t *testing.T) {
	app := New().AddSystems(Startup, Fn1(func(cmd *Commands) {
		cmd.Spawn(C(score{Value: 1}), C(score{Value: 2}))
	}))
	app.Tick(0)
	var q Query1[score]
	q.InitParam(app.World)
	if _, s, ok := q.Single(); !ok || s.Value != 2 {
		t.Fatalf("got %+v", s)
	}
}

func TestOrdering(t *testing.T) {
	var order []string
	log := func(name string) *SystemConfig {
		return Fn0(func() { order = append(order, name) }).Named(name)
	}

	const physics SystemSet = "physics"
	a, b, c := log("a"), log("b"), log("c")
	p1, p2 := log("p1").InSet(physics), log("p2").InSet(physics)
	app := New().
		AddSystems(Update, c.After(b), b.After(physics), a).
		AddSystems(Update, p1, p2).
		AddSystems(Update, Chain(log("x"), log("y"), log("z")))
	app.Tick(0)

	want := []string{"a", "p1", "p2", "b", "c", "x", "y", "z"}
	if !slices.Equal(order, want) {
		t.Fatalf("got %v, want %v", order, want)
	}
}

func TestSetOrderingAndConditions(t *testing.T) {
	var order []string
	log := func(name string) *SystemConfig {
		return Fn0(func() { order = append(order, name) }).Named(name)
	}
	const input, logic, debug SystemSet = "input", "logic", "debug"

	app := New().
		AddSystems(Update, log("l").InSet(logic), log("i").InSet(input), log("d").InSet(debug)).
		ConfigureSets(Update, logic.After(input), debug.RunIf(Not(Once())))
	app.Tick(0)
	app.Tick(0)

	want := []string{"i", "l", "i", "l", "d"}
	if !slices.Equal(order, want) {
		t.Fatalf("got %v, want %v", order, want)
	}
}

func TestCyclePanics(t *testing.T) {
	a := Fn0(func() {}).Named("a")
	b := Fn0(func() {}).Named("b").After(a)
	a.After(b)
	defer func() {
		msg, _ := recover().(string)
		if !strings.Contains(msg, "cycle") || !strings.Contains(msg, `"a"`) {
			t.Fatalf("expected cycle panic, got %q", msg)
		}
	}()
	New().AddSystems(Update, a, b).Tick(0)
}

func TestOrderingAgainstMissingSystemPanics(t *testing.T) {
	other := Fn0(func() {}).Named("other")
	defer func() {
		msg, _ := recover().(string)
		if !strings.Contains(msg, `"other"`) {
			t.Fatalf("expected missing system panic, got %q", msg)
		}
	}()
	New().AddSystems(Update, Fn0(func() {}).After(other)).Tick(0)
}

func TestRunConditions(t *testing.T) {
	runs := 0
	app := New().AddSystems(Update, Fn0(func() { runs++ }).RunIf(ResourceExists[score]()))
	app.Tick(0)
	app.InsertResource(R(&score{}))
	app.Tick(0)
	app.Tick(0)
	if runs != 2 {
		t.Fatalf("expected 2 runs, got %d", runs)
	}
}

func TestFixedTimestep(t *testing.T) {
	var fixedDeltas []time.Duration
	frames := 0
	app := New().
		AddSystems(FixedUpdate, Fn1(func(t *Res[Time]) { fixedDeltas = append(fixedDeltas, t.Get().Delta()) })).
		AddSystems(Update, Fn1(func(t *Res[Time]) {
			frames++
			if t.Get().Delta() != 25*time.Millisecond {
				panic("Update should see the frame delta")
			}
		}))
	fixed := ecs.GetResource[FixedTime](app.World)
	fixed.Timestep = 10 * time.Millisecond

	app.Tick(25 * time.Millisecond) // 2 steps, 5ms left over
	app.Tick(25 * time.Millisecond) // 3 steps, 0ms left over

	if len(fixedDeltas) != 5 || frames != 2 {
		t.Fatalf("fixed=%d frames=%d", len(fixedDeltas), frames)
	}
	for _, d := range fixedDeltas {
		if d != fixed.Timestep {
			t.Fatalf("fixed systems should see the timestep, got %v", d)
		}
	}
	if o := fixed.Overstep(); o != 0 {
		t.Fatalf("overstep %v", o)
	}
}

func TestStartupRunsOnceInOrder(t *testing.T) {
	var order []string
	app := New().
		AddSystems(PostStartup, Fn0(func() { order = append(order, "post") })).
		AddSystems(Startup, Fn0(func() { order = append(order, "startup") })).
		AddSystems(PreStartup, Fn0(func() { order = append(order, "pre") })).
		AddSystems(First, Fn0(func() { order = append(order, "first") }))
	app.Tick(0)
	app.Tick(0)
	want := []string{"pre", "startup", "post", "first", "first"}
	if !slices.Equal(order, want) {
		t.Fatalf("got %v", order)
	}
}

type counterSystem struct {
	cmd      Commands
	runs     int
	cleaned  *bool
	resource ecs.Resource[score]
}

func (s *counterSystem) Init(w *ecs.World) {
	s.cmd.InitParam(w)
	s.resource = ecs.NewResource[score](w)
}
func (s *counterSystem) Run(*ecs.World) {
	s.runs++
	s.resource.Get().Value++
	if s.runs == 3 {
		s.cmd.Exit()
	}
}
func (s *counterSystem) Cleanup(*ecs.World) { *s.cleaned = true }

type countPlugin struct{ cleaned *bool }

func (p countPlugin) Build(app *App) {
	app.InitResource(R(&score{Value: 100}))
	app.AddSystems(Update, Sys(&counterSystem{cleaned: p.cleaned}))
}

func TestPluginsStructSystemsAndExit(t *testing.T) {
	cleaned := false
	app := New().
		InsertResource(R(&score{Value: 10})).
		AddPlugins(countPlugin{cleaned: &cleaned}).
		SetRunner(func(app *App) {
			for !app.ShouldExit() {
				app.Tick(time.Millisecond)
			}
		})
	app.Run()

	if s := ecs.GetResource[score](app.World); s.Value != 13 {
		t.Fatalf("InitResource should keep the existing resource; got %d", s.Value)
	}
	if !cleaned {
		t.Fatal("Cleanup was not called")
	}
}

func TestFuncName(t *testing.T) {
	if got := Fn2(move).Name(); got != "illusion.move" {
		t.Fatalf("got %q", got)
	}
}

func TestResGetPanicsWhenMissing(t *testing.T) {
	defer func() {
		if msg, _ := recover().(string); !strings.Contains(msg, "score") {
			t.Fatalf("got %q", msg)
		}
	}()
	New().AddSystems(Update, Fn1(func(r *Res[score]) { r.Get() })).Tick(0)
}

func TestGroup(t *testing.T) {
	var order []string
	log := func(name string) *SystemConfig {
		return Fn0(func() { order = append(order, name) }).Named(name)
	}
	enabled := false
	first := log("first")
	app := New().AddSystems(Update,
		Group(log("g1"), log("g2")).After(first).RunIf(Cond0(func() bool { return enabled })),
		first,
	)
	app.Tick(0)
	enabled = true
	app.Tick(0)
	if want := []string{"first", "first", "g1", "g2"}; !slices.Equal(order, want) {
		t.Fatalf("got %v, want %v", order, want)
	}
}

func TestSetConditionsAreCheckedOncePerRun(t *testing.T) {
	var ran []string
	log := func(name string) *SystemConfig {
		return Fn0(func() { ran = append(ran, name) }).Named(name)
	}
	const intro SystemSet = "intro"
	app := New().
		AddSystems(Update, Chain(log("a").InSet(intro), log("b").InSet(intro), log("c"))).
		ConfigureSets(Update, intro.RunIf(Once()))
	app.Tick(0)
	app.Tick(0)
	// Both members of the set run on the first frame; neither on the second.
	if want := []string{"a", "b", "c", "c"}; !slices.Equal(ran, want) {
		t.Fatalf("got %v, want %v", ran, want)
	}
}
