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
	kids *illusion.Local[map[ecs.Entity][]ecs.Entity],
) {
	// Index each parent's children in one pass first. Asking the relation
	// for one parent's children scans every parent's table, so doing that for
	// each parent costs the square of how many there are.
	k := kids.Get()
	if *k == nil {
		*k = map[ecs.Entity][]ecs.Entity{}
	}
	for parent, list := range *k {
		(*k)[parent] = list[:0]
	}
	children.Each(func(child ecs.Entity, _ *Transform, _ *GlobalTransform) {
		if parent, ok := hierarchy.Parent(child); ok {
			(*k)[parent] = append((*k)[parent], child)
		}
	})
	for parent, list := range *k {
		if len(list) == 0 {
			delete(*k, parent)
		}
	}

	query := roots.Iter()
	for query.Next() {
		t, g := query.Get()
		g.Matrix = t.Matrix()
		if list, ok := (*k)[query.Entity()]; ok {
			propagateChildren(children, *k, list, g.Matrix)
		}
	}
}

func propagateChildren(
	children *illusion.Query2Where[Transform, GlobalTransform, illusion.With[illusion.ChildOf]],
	kids map[ecs.Entity][]ecs.Entity,
	list []ecs.Entity,
	parentMatrix rl.Matrix,
) {
	for _, child := range list {
		t, g, ok := children.Get(child)
		if !ok {
			continue
		}
		g.Matrix = rl.MatrixMultiply(t.Matrix(), parentMatrix)
		if grand, ok := kids[child]; ok {
			propagateChildren(children, kids, grand, g.Matrix)
		}
	}
}
