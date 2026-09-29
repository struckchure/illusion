package transform

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
)

// Propagate is the set that computes GlobalTransforms. Order systems that read
// GlobalTransform in PostUpdate after it.
const Propagate illusion.SystemSet = "transform.Propagate"

// Plugin keeps every entity's GlobalTransform in sync with its Transform and
// its parents' (see illusion.ChildOf). Entities with a Transform get a
// GlobalTransform automatically.
type Plugin struct{}

// Build implements [illusion.Plugin].
func (Plugin) Build(app *illusion.App) {
	for _, label := range []illusion.ScheduleLabel{illusion.PostStartup, illusion.PostUpdate} {
		app.AddSystems(label, illusion.Chain(
			illusion.Fn2(addGlobalTransforms).InSet(Propagate),
			illusion.Fn4(propagate).InSet(Propagate),
		))
	}
}

func addGlobalTransforms(
	q *illusion.Query1Where[Transform, illusion.Without[GlobalTransform]],
	cmd *illusion.Commands,
) {
	query := q.Iter()
	for query.Next() {
		cmd.Entity(query.Entity()).Insert(illusion.C(GlobalTransform{Matrix: query.Get().Matrix()}))
	}
}

// propagate walks each hierarchy from its roots. A child's world matrix is its
// local matrix followed by its parent's. Children of an entity without a
// Transform are not reached.
func propagate(
	roots *illusion.Query2Where[Transform, GlobalTransform, illusion.Without[illusion.ChildOf]],
	children *illusion.Query2Where[Transform, GlobalTransform, illusion.With[illusion.ChildOf]],
	hierarchy *illusion.Hierarchy,
	parents *illusion.Local[map[ecs.Entity]struct{}],
) {
	// Find which entities have children first, so childless ones (usually
	// most of them) don't each need a relation query.
	p := parents.Get()
	if *p == nil {
		*p = map[ecs.Entity]struct{}{}
	}
	clear(*p)
	children.Each(func(child ecs.Entity, _ *Transform, _ *GlobalTransform) {
		if parent, ok := hierarchy.Parent(child); ok {
			(*p)[parent] = struct{}{}
		}
	})
	if len(*p) == 0 {
		roots.Each(func(_ ecs.Entity, t *Transform, g *GlobalTransform) { g.Matrix = t.Matrix() })
		return
	}

	query := roots.Iter()
	for query.Next() {
		t, g := query.Get()
		g.Matrix = t.Matrix()
		if _, ok := (*p)[query.Entity()]; ok {
			propagateChildren(children, *p, query.Entity(), g.Matrix)
		}
	}
}

// childOfIndex is ChildOf's position in the children query: the With
// component after Transform and GlobalTransform.
const childOfIndex = 2

func propagateChildren(
	children *illusion.Query2Where[Transform, GlobalTransform, illusion.With[illusion.ChildOf]],
	parents map[ecs.Entity]struct{},
	parent ecs.Entity,
	parentMatrix rl.Matrix,
) {
	query := children.Iter(ecs.RelIdx(childOfIndex, parent))
	for query.Next() {
		t, g := query.Get()
		g.Matrix = rl.MatrixMultiply(t.Matrix(), parentMatrix)
		if _, ok := parents[query.Entity()]; ok {
			propagateChildren(children, parents, query.Entity(), g.Matrix)
		}
	}
}
