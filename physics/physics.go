package physics

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion/internal/jolt"
	"github.com/struckchure/illusion/transform"
)

// Physics is a system parameter for pushing bodies around and querying the
// world. Forces last for the next simulation step, so apply them from
// FixedUpdate. Calls on entities without a body yet (spawned this step) do
// nothing.
type Physics struct {
	world      *world
	bodies     *ecs.Map[body]
	chars      *ecs.Map[character]
	velocities *ecs.Map[Velocity]
}

// InitParam implements [illusion.Param].
func (p *Physics) InitParam(w *ecs.World) {
	p.world = ecs.GetResource[world](w)
	if p.world == nil {
		panic("physics: Physics used without physics.Plugin")
	}
	p.bodies = ecs.NewMap[body](w)
	p.chars = ecs.NewMap[character](w)
	p.velocities = ecs.NewMap[Velocity](w)
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

// Sleep puts a dynamic body to rest until an impact, force or Wake activates it.
// It clears both simulation and component velocity so Prepare cannot wake it
// again by reapplying a stale velocity.
func (p *Physics) Sleep(e ecs.Entity) {
	b := p.bodies.Get(e)
	if b == nil || b.config.rigidBody != Dynamic {
		return
	}
	p.world.jolt.Deactivate(b.id)
	b.lastVelocity = Velocity{}
	if v := p.velocities.Get(e); v != nil {
		*v = Velocity{}
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

// ignoredBody resolves either a rigid body or a character's inner body.
func (p *Physics) ignoredBody(e ecs.Entity) jolt.BodyID {
	if id, ok := p.id(e); ok {
		return id
	}
	if c := p.chars.Get(e); c != nil {
		return c.body
	}
	return jolt.InvalidBody
}

// OverlapCapsuleExcluding checks a vertical capsule for penetration.
func (p *Physics) OverlapCapsuleExcluding(center rl.Vector3, radius, height float32, exclude ecs.Entity) bool {
	if radius <= 0 || height <= 2*radius {
		return true
	}
	return p.world.jolt.OverlapCapsule(v3(center), radius, height, p.ignoredBody(exclude))
}

// SweepCapsuleExcluding checks a capsule moving by delta, returning the first obstruction.
func (p *Physics) SweepCapsuleExcluding(center, delta rl.Vector3, radius, height float32, exclude ecs.Entity) (RayHit, bool) {
	if rl.Vector3LengthSqr(delta) == 0 {
		return RayHit{}, false
	}
	h, ok := p.world.jolt.SweepCapsule(v3(center), v3(delta), radius, height, p.ignoredBody(exclude))
	if !ok {
		return RayHit{}, false
	}
	return RayHit{Entity: p.world.entities[h.Body], Point: vec(h.Point), Normal: vec(h.Normal), Distance: h.Fraction * rl.Vector3Length(delta)}, true
}

// ResizeCharacter tests clearance and preserves the feet. The prepare phase rebuilds the capsule.
func (p *Physics) ResizeCharacter(e ecs.Entity, cc *CharacterController, tr *transform.Transform, height float32) bool {
	if height <= 2*cc.Radius {
		return false
	}
	center := tr.Translation
	center.Y += (height - cc.Height) / 2
	if p.OverlapCapsuleExcluding(center, cc.Radius, height, e) {
		return false
	}
	cc.Height, tr.Translation = height, center
	return true
}

// StaticSurface reports whether e is a solid, static body suitable for authored traversal.
func (p *Physics) StaticSurface(e ecs.Entity) bool {
	b := p.bodies.Get(e)
	return b != nil && b.config.rigidBody == Static && !b.config.sensor
}
