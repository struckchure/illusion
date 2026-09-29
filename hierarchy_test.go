package illusion

import (
	"slices"
	"strings"
	"testing"

	"github.com/mlange-42/ark/ecs"
)

type name string

func spawnFamily(app *App) map[name]ecs.Entity {
	ids := map[name]ecs.Entity{}
	record := func(_ *ecs.World, e ecs.Entity) {
		ids[*ecs.NewMap[name](app.World).Get(e)] = e
	}
	app.AddSystems(Startup, Fn1(func(cmd *Commands) {
		cmd.Spawn(C(name("root"))).Then(record).WithChildren(func(c *ChildBuilder) {
			c.Spawn(C(name("a"))).Then(record).WithChildren(func(c *ChildBuilder) {
				c.Spawn(C(name("a1"))).Then(record)
			})
			c.Spawn(C(name("b"))).Then(record)
		})
		cmd.Spawn(C(name("other"))).Then(record)
	}))
	app.Tick(0)
	return ids
}

func names(w *ecs.World, es []ecs.Entity) []string {
	m := ecs.NewMap[name](w)
	out := []string{}
	for _, e := range es {
		out = append(out, string(*m.Get(e)))
	}
	slices.Sort(out)
	return out
}

func TestHierarchy(t *testing.T) {
	app := New()
	ids := spawnFamily(app)

	var h Hierarchy
	h.InitParam(app.World)
	if got := names(app.World, h.Children(ids["root"])); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("children of root: %v", got)
	}
	if p, ok := h.Parent(ids["a1"]); !ok || p != ids["a"] {
		t.Fatal("a1 should be a child of a")
	}
	if h.Root(ids["a1"]) != ids["root"] {
		t.Fatal("root of a1 should be root")
	}
	var all []ecs.Entity
	h.EachDescendant(ids["root"], func(e ecs.Entity) { all = append(all, e) })
	if got := names(app.World, all); !slices.Equal(got, []string{"a", "a1", "b"}) {
		t.Fatalf("descendants: %v", got)
	}
}

func TestReparentAndDespawnRecursive(t *testing.T) {
	app := New()
	ids := spawnFamily(app)

	// Move b under other, then despawn root: a and a1 go, b survives.
	app.AddSystems(Update, Fn1(func(cmd *Commands) {
		cmd.Entity(ids["b"]).ChildOf(ids["other"])
		cmd.Despawn(ids["root"])
	}).RunIf(Once()))
	app.Tick(0)

	for _, n := range []name{"root", "a", "a1"} {
		if app.World.Alive(ids[n]) {
			t.Fatalf("%s should be despawned", n)
		}
	}
	var h Hierarchy
	h.InitParam(app.World)
	if p, ok := h.Parent(ids["b"]); !app.World.Alive(ids["b"]) || !ok || p != ids["other"] {
		t.Fatal("b should survive under other")
	}

	app.AddSystems(Update, Fn1(func(cmd *Commands) { cmd.Entity(ids["b"]).RemoveParent() }).RunIf(Once()))
	app.Tick(0)
	if _, ok := h.Parent(ids["b"]); ok {
		t.Fatal("b should have no parent")
	}
}

func TestParentCyclePanics(t *testing.T) {
	app := New()
	ids := spawnFamily(app)
	defer func() {
		if msg, _ := recover().(string); !strings.Contains(msg, "ancestor") {
			t.Fatalf("got %q", msg)
		}
	}()
	app.AddSystems(Update, Fn1(func(cmd *Commands) { cmd.Entity(ids["root"]).ChildOf(ids["a1"]) }))
	app.Tick(0)
}

func TestCommandsOnDespawnedEntitiesAreSkipped(t *testing.T) {
	app := New()
	ids := spawnFamily(app)
	app.AddSystems(Update, Fn1(func(cmd *Commands) {
		root := ids["root"]
		cmd.Despawn(root)
		cmd.Entity(root).WithChild(C(name("orphan"))).Insert(C(score{})).ChildOf(ids["other"])
		cmd.Entity(ids["other"]).ChildOf(root)
	}).RunIf(Once()))
	app.Tick(0) // must not panic

	var q Query1[name]
	q.InitParam(app.World)
	q.Each(func(_ ecs.Entity, n *name) {
		if *n == "orphan" {
			t.Fatal("a child of a despawned parent should not be spawned")
		}
	})
	var h Hierarchy
	h.InitParam(app.World)
	if _, ok := h.Parent(ids["other"]); ok {
		t.Fatal("parenting to a despawned entity should be skipped")
	}
}
