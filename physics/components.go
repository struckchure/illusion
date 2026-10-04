// Package physics simulates rigid bodies and character controllers with Jolt
// Physics.
//
// Give an entity a [RigidBody], a [Collider] and a transform, and the plugin
// creates a body for it, steps the simulation in FixedPostUpdate and writes
// the results back into the Transform. Changing a body's RigidBody, Collider
// or optional settings (Material, Mass, ...) rebuilds it, keeping its
// velocity. Physics entities should be root entities (no ChildOf), and
// colliders ignore Transform.Scale.
package physics

import (
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
)

// RigidBody makes an entity take part in the simulation.
type RigidBody uint8

const (
	// Dynamic bodies are moved by gravity, forces and collisions.
	Dynamic RigidBody = iota
	// Static bodies never move on their own. Moving their Transform teleports
	// them.
	Static
	// Kinematic bodies follow their Transform and push dynamic bodies, but
	// nothing pushes them.
	Kinematic
)

type shapeKind uint8

const (
	shapeBox shapeKind = iota
	shapeSphere
	shapeCapsule
	shapeCylinder
	shapeConvexHull
	shapeMesh
)

// Collider is the shape a body collides with, centered on the entity unless
// offset with [Collider.WithOffset].
type Collider struct {
	kind      shapeKind
	size      [3]float32
	points    [][3]float32
	indices   []uint32
	offset    rl.Vector3
	hasOffset bool
}

// Cuboid is a box with the given full width, height and length, matching
// render.Cuboid.
func Cuboid(width, height, length float32) Collider {
	return Collider{kind: shapeBox, size: [3]float32{width / 2, height / 2, length / 2}}
}

// Sphere is a ball.
func Sphere(radius float32) Collider {
	return Collider{kind: shapeSphere, size: [3]float32{radius}}
}

// Capsule is an upright capsule with the given radius and total height
// (including both caps).
func Capsule(radius, height float32) Collider {
	return Collider{kind: shapeCapsule, size: [3]float32{radius, max(height/2-radius, 0.001)}}
}

// Cylinder is an upright cylinder centered on the entity. (render.Cylinder
// sits on its base instead; offset one of them to line them up.)
func Cylinder(radius, height float32) Collider {
	return Collider{kind: shapeCylinder, size: [3]float32{radius, height / 2}}
}

// ConvexHull is the smallest convex shape around points.
func ConvexHull(points []rl.Vector3) Collider {
	return Collider{kind: shapeConvexHull, points: vecs(points)}
}

// TriMesh is a triangle mesh, for static level geometry. indices holds three
// vertex indices per triangle.
func TriMesh(vertices []rl.Vector3, indices []uint32) Collider {
	return Collider{kind: shapeMesh, points: vecs(vertices), indices: append([]uint32(nil), indices...)}
}

// MeshCollider builds a TriMesh from a raylib mesh's vertices, e.g. a loaded
// level or a render.Mesh's embedded rl.Mesh.
func MeshCollider(m rl.Mesh) Collider {
	n := int(m.VertexCount)
	points := meshPoints(m)
	var indices []uint32
	if m.Indices != nil {
		for _, idx := range unsafe.Slice(m.Indices, int(m.TriangleCount)*3) {
			indices = append(indices, uint32(idx))
		}
	} else {
		for i := 0; i < n; i++ {
			indices = append(indices, uint32(i))
		}
	}
	return Collider{kind: shapeMesh, points: points, indices: indices}
}

// WithOffset moves the collider relative to the entity.
func (c Collider) WithOffset(offset rl.Vector3) Collider {
	c.offset, c.hasOffset = offset, true
	return c
}

func vecs(vs []rl.Vector3) [][3]float32 {
	out := make([][3]float32, len(vs))
	for i, v := range vs {
		out[i] = v3(v)
	}
	return out
}

// Sensor makes a body detect overlaps (reported as collision events) without
// colliding. A dynamic sensor still falls and takes forces, passing through
// everything; static and kinematic sensors also detect static geometry.
type Sensor struct{}

// Material sets surface properties. Without it, friction is 0.5 and
// restitution 0.
type Material struct {
	Friction    float32
	Restitution float32 // bounciness, 0..1
}

// Mass overrides the mass computed from the collider (kilograms).
type Mass float32

// GravityScale multiplies gravity for one body; 0 makes it float.
type GravityScale float32

// Damping slows a body down over time.
type Damping struct {
	Linear, Angular float32
}

// LockRotation stops a dynamic body from rotating, e.g. for upright
// characters built from rigid bodies.
type LockRotation struct{}

// ContinuousCollision stops fast, small bodies from tunneling through thin
// walls, at some cost.
type ContinuousCollision struct{}

// Velocity is a body's velocity. The plugin writes it after every step;
// change it to set the velocity.
type Velocity struct {
	Linear  rl.Vector3
	Angular rl.Vector3 // radians per second around each axis
}

// CharacterController moves an entity as a capsule that slides along walls,
// climbs slopes and steps, and stands on moving platforms. It uses the
// entity's Transform for its position and needs no RigidBody or Collider.
//
// Each fixed step, set Walk to the desired horizontal velocity; set
// Velocity.Y to jump. The plugin applies gravity and writes Velocity,
// Grounded and GroundNormal back.
type CharacterController struct {
	// Radius and Height (total, including caps) size the capsule.
	Radius, Height float32
	// MaxSlope is the steepest walkable slope in radians; 0 means 45°.
	MaxSlope float32
	// StepHeight is the tallest step the character walks up; 0 means none.
	StepHeight float32

	// Walk is the desired horizontal velocity (Y is ignored).
	Walk rl.Vector3
	// Controlled disables gravity and ground snapping, and includes Walk.Y.
	// Use for ladders and collision-tested traversal; false keeps normal walking.
	Controlled bool
	// Velocity is the character's velocity after the last step.
	Velocity rl.Vector3
	// Grounded reports whether the character stood on walkable ground after
	// the last step.
	Grounded bool
	// GroundNormal is the normal of the ground under the character.
	GroundNormal rl.Vector3
}

// CollisionStarted is sent when two entities start touching (or a sensor
// starts overlapping something).
type CollisionStarted struct {
	A, B   ecs.Entity
	Point  rl.Vector3
	Normal rl.Vector3 // from A toward B
}

// Involves reports whether e is one of the two entities, and returns the
// other one.
func (c CollisionStarted) Involves(e ecs.Entity) (other ecs.Entity, ok bool) {
	return otherOf(c.A, c.B, e)
}

// CollisionEnded is sent when two entities stop touching, including when one
// of them is despawned (A or B may then no longer be alive).
type CollisionEnded struct {
	A, B ecs.Entity
}

// Involves reports whether e is one of the two entities, and returns the
// other one.
func (c CollisionEnded) Involves(e ecs.Entity) (other ecs.Entity, ok bool) {
	return otherOf(c.A, c.B, e)
}

func otherOf(a, b, e ecs.Entity) (ecs.Entity, bool) {
	switch e {
	case a:
		return b, true
	case b:
		return a, true
	}
	return ecs.Entity{}, false
}

func v3(v rl.Vector3) [3]float32  { return [3]float32{v.X, v.Y, v.Z} }
func vec(a [3]float32) rl.Vector3 { return rl.Vector3{X: a[0], Y: a[1], Z: a[2]} }
