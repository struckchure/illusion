// Package asset stores engine assets (meshes, materials, textures, ...) and
// hands out typed handles to them.
package asset

import (
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
)

// Handle refers to an asset of type T stored in an [Assets] collection. The
// zero Handle refers to nothing.
type Handle[T any] struct {
	index      uint32
	generation uint32
}

// IsZero reports whether h is the zero handle.
func (h Handle[T]) IsZero() bool { return h.generation == 0 }

type slot[T any] struct {
	value      T
	generation uint32
	alive      bool
}

// Assets owns every asset of type T. It is a resource: request it with
// illusion.Res[asset.Assets[T]].
type Assets[T any] struct {
	slots    []slot[T]
	free     []uint32
	onRemove func(*T)
}

// New creates an empty collection. onRemove, if not nil, is called for each
// asset when it is removed or when the collection is cleared, and is where GPU
// resources get released.
func New[T any](onRemove func(*T)) *Assets[T] {
	return &Assets[T]{onRemove: onRemove}
}

// Add stores value and returns a handle to it.
func (a *Assets[T]) Add(value T) Handle[T] {
	var index uint32
	if n := len(a.free); n > 0 {
		index = a.free[n-1]
		a.free = a.free[:n-1]
	} else {
		index = uint32(len(a.slots))
		a.slots = append(a.slots, slot[T]{})
	}
	s := &a.slots[index]
	s.value = value
	s.generation++
	s.alive = true
	return Handle[T]{index: index, generation: s.generation}
}

// Get returns the asset h refers to, or nil if it was removed.
func (a *Assets[T]) Get(h Handle[T]) *T {
	if s := a.slot(h); s != nil {
		return &s.value
	}
	return nil
}

// Contains reports whether h refers to a stored asset.
func (a *Assets[T]) Contains(h Handle[T]) bool {
	return a.slot(h) != nil
}

// Remove deletes the asset h refers to. Removing a missing asset does nothing.
func (a *Assets[T]) Remove(h Handle[T]) {
	s := a.slot(h)
	if s == nil {
		return
	}
	if a.onRemove != nil {
		a.onRemove(&s.value)
	}
	var zero T
	s.value = zero
	s.alive = false
	a.free = append(a.free, h.index)
}

// Len returns the number of stored assets.
func (a *Assets[T]) Len() int {
	return len(a.slots) - len(a.free)
}

// Clear removes every asset.
func (a *Assets[T]) Clear() {
	for i := range a.slots {
		s := &a.slots[i]
		if s.alive {
			a.Remove(Handle[T]{index: uint32(i), generation: s.generation})
		}
	}
}

func (a *Assets[T]) slot(h Handle[T]) *slot[T] {
	if h.generation == 0 || int(h.index) >= len(a.slots) {
		return nil
	}
	s := &a.slots[h.index]
	if !s.alive || s.generation != h.generation {
		return nil
	}
	return s
}

// Register adds an Assets[T] resource to the app (unless one exists) and
// clears it when the app shuts down, so onRemove runs for every asset left.
func Register[T any](app *illusion.App, onRemove func(*T)) *Assets[T] {
	if existing := ecs.GetResource[Assets[T]](app.World); existing != nil {
		return existing
	}
	assets := New(onRemove)
	app.InsertResource(illusion.R(assets))
	app.AddPlugins(cleanupPlugin[T]{})
	return assets
}

type cleanupPlugin[T any] struct{}

func (cleanupPlugin[T]) Build(*illusion.App) {}

func (cleanupPlugin[T]) Cleanup(app *illusion.App) {
	if assets := ecs.GetResource[Assets[T]](app.World); assets != nil {
		assets.Clear()
	}
}
