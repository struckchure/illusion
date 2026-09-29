// Package transform positions entities in 3D space.
//
// Entities carry a [Transform] (local position, rotation, scale). The plugin
// computes a [GlobalTransform] (the final world matrix) for each of them in
// PostUpdate, and rendering reads that.
package transform

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Axis directions. Forward is -Z, matching raylib and Bevy.
var (
	Right   = rl.Vector3{X: 1}
	Up      = rl.Vector3{Y: 1}
	Forward = rl.Vector3{Z: -1}
)

// Transform is an entity's position, rotation and scale.
//
// The zero Transform has zero scale and is invisible; build one with
// [Identity], [FromXYZ] or [FromTranslation]. A zero Rotation is treated as
// no rotation.
type Transform struct {
	Translation rl.Vector3
	Rotation    rl.Quaternion
	Scale       rl.Vector3
}

// Identity is a transform at the origin with no rotation and unit scale.
func Identity() Transform {
	return Transform{Rotation: rl.QuaternionIdentity(), Scale: rl.Vector3One()}
}

// FromXYZ is an identity transform moved to (x, y, z).
func FromXYZ(x, y, z float32) Transform {
	return FromTranslation(rl.Vector3{X: x, Y: y, Z: z})
}

// FromXY is an identity transform moved to (x, y, 0), for 2D.
func FromXY(x, y float32) Transform {
	return FromXYZ(x, y, 0)
}

// FromTranslation is an identity transform moved to v.
func FromTranslation(v rl.Vector3) Transform {
	t := Identity()
	t.Translation = v
	return t
}

// FromRotation is an identity transform rotated by q.
func FromRotation(q rl.Quaternion) Transform {
	t := Identity()
	t.Rotation = q
	return t
}

// WithScale returns t uniformly scaled to s.
func (t Transform) WithScale(s float32) Transform {
	t.Scale = rl.Vector3{X: s, Y: s, Z: s}
	return t
}

// WithRotation returns t with rotation q.
func (t Transform) WithRotation(q rl.Quaternion) Transform {
	t.Rotation = q
	return t
}

// LookingAt returns t rotated so Forward points at target.
func (t Transform) LookingAt(target, up rl.Vector3) Transform {
	t.LookAt(target, up)
	return t
}

// LookAt rotates t so Forward points at target, keeping up as close to
// vertical as possible.
func (t *Transform) LookAt(target, up rl.Vector3) {
	t.Rotation = lookRotation(rl.Vector3Subtract(target, t.Translation), up)
}

// Rotate applies q on top of the current rotation (in world axes).
func (t *Transform) Rotate(q rl.Quaternion) {
	t.Rotation = rl.QuaternionNormalize(rl.QuaternionMultiply(q, t.rotation()))
}

// RotateX rotates around the world X axis by angle radians.
func (t *Transform) RotateX(angle float32) { t.Rotate(rl.QuaternionFromAxisAngle(Right, angle)) }

// RotateY rotates around the world Y axis by angle radians.
func (t *Transform) RotateY(angle float32) { t.Rotate(rl.QuaternionFromAxisAngle(Up, angle)) }

// RotateZ rotates around the world Z axis by angle radians.
func (t *Transform) RotateZ(angle float32) {
	t.Rotate(rl.QuaternionFromAxisAngle(rl.Vector3{Z: 1}, angle))
}

// Forward is the direction t faces.
func (t Transform) Forward() rl.Vector3 { return rl.Vector3RotateByQuaternion(Forward, t.rotation()) }

// Right is t's local +X direction.
func (t Transform) Right() rl.Vector3 { return rl.Vector3RotateByQuaternion(Right, t.rotation()) }

// Up is t's local +Y direction.
func (t Transform) Up() rl.Vector3 { return rl.Vector3RotateByQuaternion(Up, t.rotation()) }

// Matrix is the transform as a raylib matrix: scale, then rotate, then move.
func (t Transform) Matrix() rl.Matrix {
	m := rl.MatrixMultiply(
		rl.MatrixScale(t.Scale.X, t.Scale.Y, t.Scale.Z),
		rl.QuaternionToMatrix(t.rotation()),
	)
	m.M12, m.M13, m.M14 = t.Translation.X, t.Translation.Y, t.Translation.Z
	return m
}

func (t Transform) rotation() rl.Quaternion {
	if t.Rotation == (rl.Quaternion{}) {
		return rl.QuaternionIdentity()
	}
	return t.Rotation
}

// lookRotation builds the rotation whose Forward (-Z) points along dir.
func lookRotation(dir, up rl.Vector3) rl.Quaternion {
	if rl.Vector3LengthSqr(dir) < 1e-12 {
		return rl.QuaternionIdentity()
	}
	back := rl.Vector3Normalize(rl.Vector3Negate(dir))
	right := rl.Vector3CrossProduct(up, back)
	if rl.Vector3LengthSqr(right) < 1e-12 {
		// up is parallel to dir; pick any perpendicular axis.
		right = rl.Vector3CrossProduct(rl.Vector3{Z: 1}, back)
		if rl.Vector3LengthSqr(right) < 1e-12 {
			right = rl.Vector3CrossProduct(Right, back)
		}
	}
	right = rl.Vector3Normalize(right)
	newUp := rl.Vector3CrossProduct(back, right)

	m := rl.MatrixIdentity()
	m.M0, m.M1, m.M2 = right.X, right.Y, right.Z
	m.M4, m.M5, m.M6 = newUp.X, newUp.Y, newUp.Z
	m.M8, m.M9, m.M10 = back.X, back.Y, back.Z
	return rl.QuaternionNormalize(rl.QuaternionFromMatrix(m))
}

// GlobalTransform is an entity's final world-space matrix, computed by the
// plugin from its Transform (and, later, its parents). Treat it as read-only.
type GlobalTransform struct {
	Matrix rl.Matrix
}

// Translation is the world-space position.
func (g GlobalTransform) Translation() rl.Vector3 {
	return rl.Vector3{X: g.Matrix.M12, Y: g.Matrix.M13, Z: g.Matrix.M14}
}

// Forward is the world-space direction the entity faces.
func (g GlobalTransform) Forward() rl.Vector3 {
	return rl.Vector3Normalize(rl.Vector3{X: -g.Matrix.M8, Y: -g.Matrix.M9, Z: -g.Matrix.M10})
}

// Up is the entity's world-space +Y direction.
func (g GlobalTransform) Up() rl.Vector3 {
	return rl.Vector3Normalize(rl.Vector3{X: g.Matrix.M4, Y: g.Matrix.M5, Z: g.Matrix.M6})
}

// Right is the entity's world-space +X direction.
func (g GlobalTransform) Right() rl.Vector3 {
	return rl.Vector3Normalize(rl.Vector3{X: g.Matrix.M0, Y: g.Matrix.M1, Z: g.Matrix.M2})
}

// Decompose splits the matrix back into a Transform.
func (g GlobalTransform) Decompose() Transform {
	var t Transform
	rl.MatrixDecompose(g.Matrix, &t.Translation, &t.Rotation, &t.Scale)
	return t
}

// Deg converts degrees to radians.
func Deg(degrees float32) float32 { return degrees * math.Pi / 180 }
