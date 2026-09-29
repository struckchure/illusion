package illusion

import "github.com/mlange-42/ark/ecs"

// QueryFilter narrows a query at the type level, like Bevy's With/Without:
//
//	illusion.Query1Where[Transform, illusion.With[Player]]
//	illusion.Query2Where[Transform, Velocity, illusion.And[illusion.With[Enemy], illusion.Without[Dead]]]
type QueryFilter interface {
	terms(t *filterTerms)
}

type filterTerms struct {
	with    []ecs.Comp
	without []ecs.Comp
}

func termsOf[F QueryFilter]() filterTerms {
	var f F
	var t filterTerms
	f.terms(&t)
	return t
}

// NoFilter matches every entity that has the queried components.
type NoFilter struct{}

func (NoFilter) terms(*filterTerms) {}

// With requires entities to have component T without fetching it.
type With[T any] struct{}

func (With[T]) terms(t *filterTerms) { t.with = append(t.with, ecs.C[T]()) }

// Without excludes entities that have component T.
type Without[T any] struct{}

func (Without[T]) terms(t *filterTerms) { t.without = append(t.without, ecs.C[T]()) }

// And combines two filters. Nest it to combine more.
type And[A, B QueryFilter] struct{}

func (And[A, B]) terms(t *filterTerms) {
	var a A
	var b B
	a.terms(t)
	b.terms(t)
}

// matcher checks with/without terms against a single entity.
type matcher struct {
	with    []ecs.ID
	without []ecs.ID
}

func newMatcher(w *ecs.World, t filterTerms) matcher {
	m := matcher{}
	for _, c := range t.with {
		m.with = append(m.with, ecs.TypeID(w, c.Type()))
	}
	for _, c := range t.without {
		m.without = append(m.without, ecs.TypeID(w, c.Type()))
	}
	return m
}

func (m *matcher) matches(w *ecs.World, e ecs.Entity) bool {
	u := w.Unsafe()
	for _, id := range m.with {
		if !u.Has(e, id) {
			return false
		}
	}
	for _, id := range m.without {
		if u.Has(e, id) {
			return false
		}
	}
	return true
}
