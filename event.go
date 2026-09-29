package illusion

import (
	"iter"

	"github.com/mlange-42/ark/ecs"
)

// Events stores events of type T. Events live for two frames: every reader
// sees each event once, whether it runs before or after the sender in the
// frame. They are dropped at the start of the frame after next.
//
// Systems use [EventWriter] and [EventReader]; Events is the resource behind
// them, for code outside systems.
type Events[T any] struct {
	older, newer           []T
	olderStart, newerStart uint64 // sequence number of each buffer's first event
}

// Send adds an event.
func (e *Events[T]) Send(event T) {
	e.newer = append(e.newer, event)
}

// Len returns the number of stored events.
func (e *Events[T]) Len() int {
	return len(e.older) + len(e.newer)
}

func (e *Events[T]) end() uint64 {
	return e.newerStart + uint64(len(e.newer))
}

// update drops the older buffer and starts a new one. It runs at the start of
// every frame.
func (e *Events[T]) update() {
	clear(e.older)
	e.older, e.newer = e.newer, e.older[:0]
	e.olderStart = e.newerStart
	e.newerStart = e.olderStart + uint64(len(e.older))
}

// at returns the event with sequence number id, which must be stored.
func (e *Events[T]) at(id uint64) T {
	if id < e.newerStart {
		return e.older[id-e.olderStart]
	}
	return e.newer[id-e.newerStart]
}

// eventRegistry updates every Events resource once per frame.
type eventRegistry struct {
	updaters []func()
}

func (r *eventRegistry) update() {
	for _, u := range r.updaters {
		u()
	}
}

// eventsFor returns the Events[T] resource, creating and registering it on
// first use.
func eventsFor[T any](w *ecs.World) *Events[T] {
	res := ecs.NewResource[Events[T]](w)
	if ev := res.Get(); ev != nil {
		return ev
	}
	ev := &Events[T]{}
	res.Add(ev)
	reg := ecs.GetResource[eventRegistry](w)
	reg.updaters = append(reg.updaters, ev.update)
	return ev
}

// AddEvent registers event type T up front, so the Events[T] resource exists
// before any system that uses it runs. Readers and writers register their
// event type automatically, so this is only needed to use Events[T] directly.
func AddEvent[T any](app *App) *App {
	eventsFor[T](app.World)
	return app
}

// EventWriter is a system parameter for sending events of type T.
type EventWriter[T any] struct {
	events *Events[T]
}

// InitParam implements [Param].
func (w *EventWriter[T]) InitParam(world *ecs.World) {
	w.events = eventsFor[T](world)
}

// Send sends an event.
func (w *EventWriter[T]) Send(event T) {
	w.events.Send(event)
}

// SendBatch sends several events.
func (w *EventWriter[T]) SendBatch(events ...T) {
	w.events.newer = append(w.events.newer, events...)
}

// EventReader is a system parameter for reading events of type T. Each reader
// remembers what it has read, so every system sees each event exactly once.
type EventReader[T any] struct {
	events *Events[T]
	next   uint64
}

// InitParam implements [Param].
func (r *EventReader[T]) InitParam(world *ecs.World) {
	r.events = eventsFor[T](world)
	r.next = r.events.olderStart
}

// Read returns the events this reader hasn't seen yet and marks them read:
//
//	for hit := range hits.Read() { ... }
func (r *EventReader[T]) Read() iter.Seq[T] {
	return func(yield func(T) bool) {
		start, end := r.unread()
		for id := start; id < end; id++ {
			r.next = id + 1
			if !yield(r.events.at(id)) {
				return
			}
		}
		r.next = max(r.next, end)
	}
}

// Len returns the number of unread events.
func (r *EventReader[T]) Len() int {
	start, end := r.unread()
	return int(end - start)
}

// IsEmpty reports whether there are no unread events.
func (r *EventReader[T]) IsEmpty() bool {
	return r.Len() == 0
}

// Clear marks every stored event as read.
func (r *EventReader[T]) Clear() {
	r.next = r.events.end()
}

func (r *EventReader[T]) unread() (start, end uint64) {
	return max(r.next, r.events.olderStart), r.events.end()
}
