package illusion

import "github.com/mlange-42/ark/ecs"

// ResourceExists holds while a resource of type T exists.
func ResourceExists[T any]() Condition {
	return Cond1(func(r *Res[T]) bool { return r.Has() })
}

// Not inverts a condition.
func Not(c Condition) Condition {
	return &notCondition{inner: c}
}

type notCondition struct{ inner Condition }

func (n *notCondition) Init(w *ecs.World)       { n.inner.Init(w) }
func (n *notCondition) Check(w *ecs.World) bool { return !n.inner.Check(w) }

// Once holds only the first time it is checked. Each call returns a new
// condition with its own state; don't share one between schedules (or
// between systems, unless you mean "only the first of them"). On a set it is
// checked once for the whole set.
func Once() Condition {
	return Cond1(func(ran *Local[bool]) bool {
		if *ran.Get() {
			return false
		}
		*ran.Get() = true
		return true
	})
}
