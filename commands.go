package illusion

import (
	"slices"
	"unsafe"

	"github.com/mlange-42/ark/ecs"
)

// Component is a component value waiting to be added to an entity.
// Create one with [C].
type Component interface {
	componentID(w *ecs.World) ecs.ID
	write(dst unsafe.Pointer)
}

type componentValue[T any] struct {
	value T
}

// C packages a component value for [Commands.Spawn] and [EntityCommands.Insert].
func C[T any](value T) Component {
	return componentValue[T]{value: value}
}

func (c componentValue[T]) componentID(w *ecs.World) ecs.ID { return ecs.ComponentID[T](w) }
func (c componentValue[T]) write(dst unsafe.Pointer)        { *(*T)(dst) = c.value }

// Resource is a resource value waiting to be inserted into the world.
// Create one with [R].
type Resource interface {
	insert(w *ecs.World, replace bool)
}

type resourceValue[T any] struct {
	ptr *T
}

// R packages a resource for [App.InsertResource] and [Commands.InsertResource].
func R[T any](ptr *T) Resource {
	return resourceValue[T]{ptr: ptr}
}

func (r resourceValue[T]) insert(w *ecs.World, replace bool) {
	res := ecs.NewResource[T](w)
	if res.Has() {
		if !replace {
			return
		}
		res.Remove()
	}
	res.Add(r.ptr)
}

// Commands queues changes to the world. They are applied right after the
// system that queued them finishes, so they are safe to issue while iterating
// a query, when Ark's world is locked.
//
// Commands for entities that no longer exist when the queue is applied
// (despawned by an earlier command, say) are skipped, including spawning a
// child of a despawned parent.
//
// Commands is a system parameter. The *EntityCommands it returns are only
// valid until the queue is applied; don't keep them.
type Commands struct {
	queue *commandQueue
}

// InitParam implements [Param].
func (c *Commands) InitParam(w *ecs.World) {
	c.queue = ecs.GetResource[commandQueue](w)
	if c.queue == nil {
		panic("illusion: Commands used on a world that was not created by illusion.New")
	}
}

// Spawn queues a new entity with the given components.
func (c *Commands) Spawn(components ...Component) EntityCommands {
	slot := c.queue.newSlot(ecs.Entity{})
	c.queue.push(command{kind: cmdSpawn, slot: slot, components: components})
	return EntityCommands{queue: c.queue, slot: slot}
}

// Entity returns commands for an existing entity.
func (c *Commands) Entity(e ecs.Entity) EntityCommands {
	return EntityCommands{queue: c.queue, slot: c.queue.newSlot(e)}
}

// Despawn queues removal of e and all its descendants. Despawning a dead
// entity does nothing.
func (c *Commands) Despawn(e ecs.Entity) {
	c.Entity(e).Despawn()
}

// InsertResource queues inserting (or replacing) a resource.
func (c *Commands) InsertResource(r Resource) {
	c.queue.push(command{kind: cmdWorld, worldFn: func(w *ecs.World) { r.insert(w, true) }})
}

// Queue runs fn with the world when the queue is applied.
func (c *Commands) Queue(fn func(w *ecs.World)) {
	c.queue.push(command{kind: cmdWorld, worldFn: fn})
}

// Exit asks the app to stop after the current frame.
func (c *Commands) Exit() {
	c.Queue(func(w *ecs.World) { ecs.GetResource[AppExit](w).Requested = true })
}

// EntityCommands queues changes to one entity, which may not be spawned yet.
type EntityCommands struct {
	queue *commandQueue
	slot  int
}

// Insert queues adding components. Components the entity already has are
// overwritten.
func (e EntityCommands) Insert(components ...Component) EntityCommands {
	e.queue.push(command{kind: cmdInsert, slot: e.slot, components: components})
	return e
}

// Remove queues removing components. Components the entity lacks are ignored.
func (e EntityCommands) Remove(components ...ecs.Comp) EntityCommands {
	e.queue.push(command{kind: cmdRemove, slot: e.slot, remove: components})
	return e
}

// Despawn queues removing the entity and all its descendants.
func (e EntityCommands) Despawn() {
	e.queue.push(command{kind: cmdDespawn, slot: e.slot})
}

// ChildOf queues making the entity a child of parent, replacing any previous
// parent.
func (e EntityCommands) ChildOf(parent ecs.Entity) EntityCommands {
	e.queue.push(command{kind: cmdSetParent, slot: e.slot, parent: e.queue.newSlot(parent)})
	return e
}

// RemoveParent queues detaching the entity from its parent.
func (e EntityCommands) RemoveParent() EntityCommands {
	return e.Remove(ecs.C[ChildOf]())
}

// WithChildren calls fn to spawn children of this entity.
func (e EntityCommands) WithChildren(fn func(children *ChildBuilder)) EntityCommands {
	fn(&ChildBuilder{queue: e.queue, parent: e.slot})
	return e
}

// WithChild queues a single child with the given components.
func (e EntityCommands) WithChild(components ...Component) EntityCommands {
	(&ChildBuilder{queue: e.queue, parent: e.slot}).Spawn(components...)
	return e
}

// Then queues fn to run with the real entity once it exists. Use it to get
// hold of a spawned entity, e.g. to store it in a resource.
func (e EntityCommands) Then(fn func(w *ecs.World, entity ecs.Entity)) EntityCommands {
	e.queue.push(command{kind: cmdEntity, slot: e.slot, entityFn: fn})
	return e
}

type commandKind uint8

const (
	cmdSpawn commandKind = iota
	cmdInsert
	cmdRemove
	cmdDespawn
	cmdEntity
	cmdWorld
	cmdSetParent
)

type command struct {
	kind       commandKind
	slot       int
	hasParent  bool
	parent     int
	components []Component
	remove     []ecs.Comp
	entityFn   func(*ecs.World, ecs.Entity)
	worldFn    func(*ecs.World)
}

type commandQueue struct {
	commands []command
	slots    []ecs.Entity

	// Scratch space for resolving components.
	comps []Component
	ids   []ecs.ID
	added []ecs.ID

	// Hierarchy support, created on first use.
	hierarchyReady bool
	childOf        ecs.ID
	children       *ecs.Filter0
}

func (q *commandQueue) initHierarchy(w *ecs.World) {
	if !q.hierarchyReady {
		q.hierarchyReady = true
		q.childOf = ecs.ComponentID[ChildOf](w)
		q.children = childFilter(w)
	}
}

func (q *commandQueue) newSlot(e ecs.Entity) int {
	q.slots = append(q.slots, e)
	return len(q.slots) - 1
}

func (q *commandQueue) push(c command) {
	q.commands = append(q.commands, c)
}

// apply runs every queued command in order. Commands queued while applying
// (from Queue or Then callbacks) run in the same pass.
func (q *commandQueue) apply(w *ecs.World) {
	for i := 0; i < len(q.commands); i++ {
		c := q.commands[i]
		switch c.kind {
		case cmdSpawn:
			if c.hasParent {
				q.slots[c.slot] = q.spawnChild(w, c.components, q.slots[c.parent])
			} else {
				q.slots[c.slot] = q.spawn(w, c.components)
			}
		case cmdInsert:
			q.insert(w, q.slots[c.slot], c.components)
		case cmdRemove:
			q.removeComponents(w, q.slots[c.slot], c.remove)
		case cmdDespawn:
			if e := q.slots[c.slot]; w.Alive(e) {
				q.despawn(w, e)
			}
		case cmdSetParent:
			q.setParent(w, q.slots[c.slot], q.slots[c.parent])
		case cmdEntity:
			if e := q.slots[c.slot]; w.Alive(e) {
				c.entityFn(w, e)
			}
		case cmdWorld:
			c.worldFn(w)
		}
		q.commands[i] = command{}
	}
	q.commands = q.commands[:0]
	q.slots = q.slots[:0]
}

// resolve looks up each component's ID once, keeping the last value of any
// type given twice, into the parallel q.comps and q.ids.
func (q *commandQueue) resolve(w *ecs.World, components []Component) {
	q.comps = q.comps[:0]
	q.ids = q.ids[:0]
	for _, c := range components {
		id := c.componentID(w)
		if k := slices.Index(q.ids, id); k >= 0 {
			q.comps[k] = c
			continue
		}
		q.ids = append(q.ids, id)
		q.comps = append(q.comps, c)
	}
}

// write copies the resolved values into e.
func (q *commandQueue) write(u ecs.Unsafe, e ecs.Entity) {
	for i, c := range q.comps {
		c.write(u.Get(e, q.ids[i]))
	}
	clear(q.comps) // don't hold on to the values
}

// spawn creates the entity with all components in one archetype move.
//
// Ark's OnCreateEntity observers fire before the values are written, so they
// see zero values. Use Then, or a typed ecs.Map, if an observer needs them.
func (q *commandQueue) spawn(w *ecs.World, components []Component) ecs.Entity {
	q.resolve(w, components)
	u := w.Unsafe()
	e := u.NewEntity(q.ids...)
	q.write(u, e)
	return e
}

// spawnChild spawns with the ChildOf relation set, in one move. If the parent
// is gone (e.g. despawned earlier in the queue), nothing is spawned: the child
// would have been despawned with it.
func (q *commandQueue) spawnChild(w *ecs.World, components []Component, parent ecs.Entity) ecs.Entity {
	if !w.Alive(parent) {
		return ecs.Entity{}
	}
	q.initHierarchy(w)
	q.resolve(w, components)
	u := w.Unsafe()
	e := u.NewEntityRel(append(q.ids, q.childOf), ecs.RelID(q.childOf, parent))
	q.write(u, e)
	return e
}

func (q *commandQueue) setParent(w *ecs.World, e, parent ecs.Entity) {
	if !w.Alive(e) || !w.Alive(parent) {
		return
	}
	q.initHierarchy(w)
	u := w.Unsafe()
	for a := parent; ; {
		if a == e {
			panic("illusion: ChildOf would make an entity its own ancestor")
		}
		if !u.Has(a, q.childOf) {
			break
		}
		a = u.GetRelation(a, q.childOf)
	}
	rel := ecs.RelID(q.childOf, parent)
	if u.Has(e, q.childOf) {
		u.SetRelations(e, rel)
	} else {
		u.AddRel(e, []ecs.ID{q.childOf}, rel)
	}
}

// despawn removes e after its descendants.
func (q *commandQueue) despawn(w *ecs.World, e ecs.Entity) {
	q.initHierarchy(w)
	var children []ecs.Entity
	query := q.children.Query(ecs.RelIdx(0, e))
	for query.Next() {
		children = append(children, query.Entity())
	}
	for _, child := range children {
		q.despawn(w, child)
	}
	w.RemoveEntity(e)
}

func (q *commandQueue) insert(w *ecs.World, e ecs.Entity, components []Component) {
	if !w.Alive(e) {
		return
	}
	q.resolve(w, components)
	u := w.Unsafe()
	q.added = q.added[:0]
	for _, id := range q.ids {
		if !u.Has(e, id) {
			q.added = append(q.added, id)
		}
	}
	if len(q.added) > 0 {
		u.Add(e, q.added...)
	}
	q.write(u, e)
}

func (q *commandQueue) removeComponents(w *ecs.World, e ecs.Entity, comps []ecs.Comp) {
	if !w.Alive(e) {
		return
	}
	u := w.Unsafe()
	q.ids = q.ids[:0]
	for _, c := range comps {
		if id := ecs.TypeID(w, c.Type()); u.Has(e, id) {
			q.ids = append(q.ids, id)
		}
	}
	if len(q.ids) > 0 {
		u.Remove(e, q.ids...)
	}
}

// AppExit records whether the app has been asked to stop.
type AppExit struct {
	Requested bool
}
