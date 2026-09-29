package illusion

import "github.com/mlange-42/ark/ecs"

// ChildOf is the Ark relation that makes an entity the child of its target.
// Set it with [EntityCommands.ChildOf] or [EntityCommands.WithChildren].
//
// Despawning a parent despawns its children too.
type ChildOf struct {
	ecs.RelationMarker
}

// childFilter builds the filter used to find the children of a parent: pass
// ecs.RelIdx(0, parent) to its Query.
func childFilter(w *ecs.World) *ecs.Filter0 {
	return ecs.NewFilter0(w).With(ecs.C[ChildOf]()).Register()
}

// Hierarchy is a system parameter for walking parent/child relations.
type Hierarchy struct {
	world    *ecs.World
	parents  *ecs.Map[ChildOf]
	children *ecs.Filter0
}

// InitParam implements [Param].
func (h *Hierarchy) InitParam(w *ecs.World) {
	h.world = w
	h.parents = ecs.NewMap[ChildOf](w)
	h.children = childFilter(w)
}

// Parent returns e's parent, if it has one.
func (h *Hierarchy) Parent(e ecs.Entity) (ecs.Entity, bool) {
	if !h.world.Alive(e) || !h.parents.Has(e) {
		return ecs.Entity{}, false
	}
	return h.parents.GetRelation(e), true
}

// Root returns the topmost ancestor of e (e itself if it has no parent).
func (h *Hierarchy) Root(e ecs.Entity) ecs.Entity {
	for {
		parent, ok := h.Parent(e)
		if !ok {
			return e
		}
		e = parent
	}
}

// EachChild calls fn for each direct child of parent.
func (h *Hierarchy) EachChild(parent ecs.Entity, fn func(child ecs.Entity)) {
	query := h.children.Query(ecs.RelIdx(0, parent))
	for query.Next() {
		fn(query.Entity())
	}
}

// Children returns the direct children of parent.
func (h *Hierarchy) Children(parent ecs.Entity) []ecs.Entity {
	var out []ecs.Entity
	h.EachChild(parent, func(child ecs.Entity) { out = append(out, child) })
	return out
}

// EachDescendant calls fn for every descendant of parent, parents before
// their children.
func (h *Hierarchy) EachDescendant(parent ecs.Entity, fn func(e ecs.Entity)) {
	for _, child := range h.Children(parent) {
		fn(child)
		h.EachDescendant(child, fn)
	}
}

// ChildBuilder spawns children of one parent. Get one from
// [EntityCommands.WithChildren].
type ChildBuilder struct {
	queue  *commandQueue
	parent int
}

// Spawn queues a new child with the given components.
func (b *ChildBuilder) Spawn(components ...Component) EntityCommands {
	slot := b.queue.newSlot(ecs.Entity{})
	b.queue.push(command{kind: cmdSpawn, slot: slot, components: components, hasParent: true, parent: b.parent})
	return EntityCommands{queue: b.queue, slot: slot}
}

// Parent returns commands for the parent being built.
func (b *ChildBuilder) Parent() EntityCommands {
	return EntityCommands{queue: b.queue, slot: b.parent}
}
