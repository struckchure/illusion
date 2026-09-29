package illusion

import (
	"fmt"

	"github.com/mlange-42/ark/ecs"
)

// Param is a value that a function system receives as an argument.
//
// Each system owns one instance of every parameter it declares. The instance is
// initialized once, when the system is added to a running schedule, and is then
// reused on every run.
//
// Implement Param on your own struct to build custom parameters, e.g. a bundle
// of queries and resources that several systems share. Call InitParam on the
// embedded parameters from your own InitParam.
type Param interface {
	InitParam(w *ecs.World)
}

// paramPtr is the constraint used by the generated system adapters. It lets
// them allocate a P by value and still call the pointer-receiver InitParam.
type paramPtr[T any] interface {
	*T
	Param
}

// Res gives a system access to a resource of type T.
type Res[T any] struct {
	res ecs.Resource[T]
}

// InitParam implements [Param].
func (r *Res[T]) InitParam(w *ecs.World) {
	r.res = ecs.NewResource[T](w)
}

// Get returns the resource. It panics if the resource does not exist.
func (r *Res[T]) Get() *T {
	v := r.res.Get()
	if v == nil {
		panic(fmt.Sprintf("illusion: resource %T does not exist", v))
	}
	return v
}

// TryGet returns the resource and whether it exists.
func (r *Res[T]) TryGet() (*T, bool) {
	v := r.res.Get()
	return v, v != nil
}

// Has reports whether the resource exists.
func (r *Res[T]) Has() bool {
	return r.res.Has()
}

// Local is state private to a single system. It persists between runs and
// starts as the zero value of T.
type Local[T any] struct {
	value T
}

// InitParam implements [Param].
func (l *Local[T]) InitParam(*ecs.World) {}

// Get returns a pointer to the system's local value.
func (l *Local[T]) Get() *T {
	return &l.value
}

// World gives a system direct access to the Ark world. Use it for anything the
// other parameters don't cover, like batch operations or Ark observers.
//
// Structural changes (creating entities, adding or removing components) are
// fine here as long as no query is open. Prefer [Commands] inside query loops.
type World struct {
	*ecs.World
}

// InitParam implements [Param].
func (p *World) InitParam(w *ecs.World) {
	p.World = w
}
