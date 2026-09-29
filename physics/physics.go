package physics

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion/internal/jolt"
)

// Physics is a system parameter for pushing bodies around and querying the
// world. Forces last for the next simulation step, so apply them from
// FixedUpdate. Calls on entities without a body yet (spawned this step) do
// nothing.
type Physics struct {
	world  *world
	bodies *ecs.Map[body]
	chars  *ecs.Map[character]
}

// InitParam implements [illusion.Param].
func (p *Physics) InitParam(w *ecs.World) {
	p.world = ecs.GetResource[world](w)
	if p.world == nil {
		panic("physics: Physics used without physics.Plugin")
	}
	p.bodies = ecs.NewMap[body](w)
	p.chars = ecs.NewMap[character](w)
}

func (p *Physics) id(e ecs.Entity) (jolt.BodyID, bool) {
	if b := p.bodies.Get(e); b != nil {
		return b.id, true
	}
	return 0, false
}

// AddForce pushes e's center of mass with force (Newtons) for the next step.
func (p *Physics) AddForce(e ecs.Entity, force rl.Vector3) {
	if id, ok := p.id(e); ok {
		p.world.jolt.AddForce(id, v3(force))
	}
}

// AddTorque twists e for the next step.
func (p *Physics) AddTorque(e ecs.Entity, torque rl.Vector3) {
	if id, ok := p.id(e); ok {
		p.world.jolt.AddTorque(id, v3(torque))
	}
}

// AddImpulse changes e's momentum immediately, like a kick or an explosion.
func (p *Physics) AddImpulse(e ecs.Entity, impulse rl.Vector3) {
	if id, ok := p.id(e); ok {
		p.world.jolt.AddImpulse(id, v3(impulse))
	}
}

// AddAngularImpulse changes e's spin immediately.
func (p *Physics) AddAngularImpulse(e ecs.Entity, impulse rl.Vector3) {
	if id, ok := p.id(e); ok {
		p.world.jolt.AddAngularImpulse(id, v3(impulse))
	}
}

// Wake makes a sleeping body simulate again.
func (p *Physics) Wake(e ecs.Entity) {
	if id, ok := p.id(e); ok {
		p.world.jolt.Activate(id)
	}
}

// Asleep reports whether e's body has come to rest and stopped simulating.
func (p *Physics) Asleep(e ecs.Entity) bool {
	id, ok := p.id(e)
	return ok && !p.world.jolt.IsActive(id)
}

// RayHit describes where a ray hit.
type RayHit struct {
	Entity   ecs.Entity
	Point    rl.Vector3
	Normal   rl.Vector3
	Distance float32
}

// CastRay finds the closest entity along a ray from origin in direction, up
// to maxDistance away.
func (p *Physics) CastRay(origin, direction rl.Vector3, maxDistance float32) (RayHit, bool) {
	return p.cast(origin, direction, maxDistance, jolt.InvalidBody)
}

// CastRayExcluding is CastRay ignoring one entity, typically the caster.
func (p *Physics) CastRayExcluding(origin, direction rl.Vector3, maxDistance float32, exclude ecs.Entity) (RayHit, bool) {
	ignore := jolt.InvalidBody
	if id, ok := p.id(exclude); ok {
		ignore = id
	} else if c := p.chars.Get(exclude); c != nil {
		ignore = c.body
	}
	return p.cast(origin, direction, maxDistance, ignore)
}

func (p *Physics) cast(origin, direction rl.Vector3, maxDistance float32, ignore jolt.BodyID) (RayHit, bool) {
	if rl.Vector3LengthSqr(direction) == 0 {
		return RayHit{}, false
	}
	dir := rl.Vector3Scale(rl.Vector3Normalize(direction), maxDistance)
	hit, ok := p.world.jolt.CastRay(v3(origin), v3(dir), ignore)
	if !ok {
		return RayHit{}, false
	}
	e, known := p.world.entities[hit.Body]
	if !known {
		return RayHit{}, false
	}
	return RayHit{Entity: e, Point: vec(hit.Point), Normal: vec(hit.Normal), Distance: hit.Fraction * maxDistance}, true
}
