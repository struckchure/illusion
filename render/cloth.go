package render

import (
	"math"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Cloth lets parts of an entity's skinned Model3d move with physics: hair
// that swings, a skirt that sways and trails. Each simulated vertex follows
// where the animation puts it, but it has weight: it lags behind when the
// entity moves, hangs under gravity, keeps its distance to its neighbours,
// and is pushed out of the Colliders. Freedom limits how far it can stray,
// and it never goes in behind where the animation puts it (along the
// surface's normal there), so it can't sink into the body or what's worn
// under it.
//
// Cloth is simulated when the entity is drawn, where it poses the model
// too, so it needs an AnimationPlayer. Its state lives in the component:
// a fresh Cloth starts where the pose is.
type Cloth struct {
	// Meshes are the simulated meshes, by index in the model. The rest are
	// only skinned.
	Meshes map[int]ClothMesh
	// Colliders push the cloth out; they move with the model's bones.
	Colliders []Capsule
	// Gravity is the acceleration on the cloth, in world space. The zero
	// value means 9.8 m/s² down.
	Gravity rl.Vector3
	// Damping is the share of its speed the cloth loses each 60th of a
	// second (0 to 1). The zero value means 0.03.
	Damping float32
	// Stiffness is the share of the way back to its animated shape the cloth
	// is pulled each 60th of a second (0 to 1). The zero value means 0.02.
	Stiffness float32
	// Thickness is how far the cloth keeps off the Colliders, in metres.
	// The zero value means 0.005.
	Thickness float32

	state *clothState
}

// ClothMesh says how one mesh moves.
type ClothMesh struct {
	// Freedom is, for each vertex of the mesh (in the mesh's order), how far
	// in metres it can move from where the animation puts it; 0 pins it
	// there. Vertices at the same place (split for texture seams) move as
	// one, with the most freedom among them.
	Freedom []float32
}

// Capsule is a collider: a segment and the radius around it. Its ends A and
// B are points in the model's bind pose, carried by bones BoneA and BoneB
// (by index in its skeleton) as they move. Equal ends make a sphere.
type Capsule struct {
	A, B         rl.Vector3
	BoneA, BoneB int
	Radius       float32
}

const (
	clothStep       = float32(1.0 / 60) // the simulation's time step, in seconds
	clothMaxSteps   = 4                 // steps per frame at most; a slower frame runs slow
	clothIterations = 4                 // constraint passes per step
	clothTeleport   = 1.0               // metres: moving further in a frame restarts the cloth
)

type clothState struct {
	meshes map[int]*clothMesh
	posed  []posedMesh // every skinned mesh as posed this frame
	bones  []rl.Matrix // bone matrices for this frame's pose
	origin rl.Vector3  // the model's origin last frame, in world space
	spare  float32     // simulated time owed, in seconds
}

// clothMesh is one mesh's simulation. Vertices at the same bind position
// share a particle.
type clothMesh struct {
	particle []int32   // vertex -> particle
	first    []int32   // particle -> one of its vertices
	freedom  []float32 // particle -> how far it can stray
	edges    [][2]int32
	free     []int32 // the particles that move

	pos, prev          []rl.Vector3 // particles, world space
	target, lastTarget []rl.Vector3 // where the pose puts each particle, world space
	normal             []rl.Vector3 // the posed surface's normal at each moving particle, world space
	started            bool
}

// newClothMesh builds the simulation of a mesh with vertices (bind
// positions) and triangles (vertex indices, three a triangle).
func newClothMesh(vertices []rl.Vector3, triangles []int32, freedom []float32) *clothMesh {
	c := &clothMesh{particle: make([]int32, len(vertices))}
	at := map[rl.Vector3]int32{}
	for i, v := range vertices {
		p, ok := at[v]
		if !ok {
			p = int32(len(c.first))
			at[v] = p
			c.first = append(c.first, int32(i))
			c.freedom = append(c.freedom, 0)
		}
		c.particle[i] = p
		if i < len(freedom) && freedom[i] > c.freedom[p] {
			c.freedom[p] = freedom[i]
		}
	}
	seen := map[[2]int32]bool{}
	for t := 0; t+2 < len(triangles); t += 3 {
		for k := range 3 {
			a, b := c.particle[triangles[t+k]], c.particle[triangles[t+(k+1)%3]]
			if a == b || (c.freedom[a] == 0 && c.freedom[b] == 0) {
				continue
			}
			if a > b {
				a, b = b, a
			}
			if e := [2]int32{a, b}; !seen[e] {
				seen[e] = true
				c.edges = append(c.edges, e)
			}
		}
	}
	for p, f := range c.freedom {
		if f > 0 {
			c.free = append(c.free, int32(p))
		}
	}
	n := len(c.first)
	c.pos = make([]rl.Vector3, n)
	c.prev = make([]rl.Vector3, n)
	c.target = make([]rl.Vector3, n)
	c.lastTarget = make([]rl.Vector3, n)
	c.normal = make([]rl.Vector3, n)
	return c
}

// skin poses the mesh's bind vertices like raylib's CPU skinning does.
func skin(out []rl.Vector3, vertices []rl.Vector3, boneIDs []uint8, weights []float32, bones []rl.Matrix) {
	for i, v := range vertices {
		var s rl.Vector3
		for j := range 4 {
			w := weights[4*i+j]
			if w == 0 {
				continue
			}
			p := rl.Vector3Transform(v, bones[boneIDs[4*i+j]])
			s.X += p.X * w
			s.Y += p.Y * w
			s.Z += p.Z * w
		}
		out[i] = s
	}
}

// skinNormal poses normal n of a vertex with boneIDs and weights (its four
// each), turning it with the bones.
func skinNormal(n rl.Vector3, boneIDs []uint8, weights []float32, bones []rl.Matrix) rl.Vector3 {
	var s rl.Vector3
	for j := range 4 {
		if w := weights[j]; w != 0 {
			m := bones[boneIDs[j]]
			m.M12, m.M13, m.M14 = 0, 0, 0
			s = rl.Vector3Add(s, rl.Vector3Scale(rl.Vector3Transform(n, m), w))
		}
	}
	return s
}

// worldCapsule is a Capsule placed in world space for this frame.
type worldCapsule struct {
	a, b   rl.Vector3
	radius float32
}

// step advances the simulation by h seconds, the targets moving from
// lastTarget to target over the step's share t of the frame.
func (c *clothMesh) step(h, t float32, gravity rl.Vector3, keep, stiffness float32, colliders []worldCapsule) {
	g := rl.Vector3Scale(gravity, h*h)
	for _, p := range c.free {
		target := rl.Vector3Lerp(c.lastTarget[p], c.target[p], t)
		x := c.pos[p]
		v := rl.Vector3Scale(rl.Vector3Subtract(x, c.prev[p]), keep)
		c.prev[p] = x
		x = rl.Vector3Add(rl.Vector3Add(x, v), g)
		c.pos[p] = rl.Vector3Lerp(x, target, stiffness)
	}
	for range clothIterations {
		for _, e := range c.edges {
			a, b := e[0], e[1]
			pa, pb := c.at(a, t), c.at(b, t)
			d := rl.Vector3Subtract(pb, pa)
			length := rl.Vector3Length(d)
			if length < 1e-6 {
				continue
			}
			rest := rl.Vector3Distance(c.targetAt(a, t), c.targetAt(b, t))
			wa, wb := c.mobility(a), c.mobility(b)
			if wa+wb == 0 {
				continue
			}
			fix := rl.Vector3Scale(d, (length-rest)/length/(wa+wb))
			if wa > 0 {
				c.pos[a] = rl.Vector3Add(pa, rl.Vector3Scale(fix, wa))
			}
			if wb > 0 {
				c.pos[b] = rl.Vector3Subtract(pb, rl.Vector3Scale(fix, wb))
			}
		}
		for _, p := range c.free {
			target := c.targetAt(p, t)
			// Within its freedom of where the pose puts it, and not behind it.
			off := rl.Vector3Subtract(c.pos[p], target)
			if rl.Vector3Length(off) > c.freedom[p] {
				off = rl.Vector3Scale(rl.Vector3Normalize(off), c.freedom[p])
			}
			if in := rl.Vector3DotProduct(off, c.normal[p]); in < 0 {
				off = rl.Vector3Subtract(off, rl.Vector3Scale(c.normal[p], in))
			}
			// And out of the body, whatever the pose says: the pose sinks
			// loose cloth into a leg that swings into it.
			c.pos[p] = collide(rl.Vector3Add(target, off), target, colliders)
		}
	}
}

// mobility is 1 for a moving particle, 0 for a pinned one.
func (c *clothMesh) mobility(p int32) float32 {
	if c.freedom[p] > 0 {
		return 1
	}
	return 0
}

// at is where particle p is: pinned ones are wherever the pose puts them.
func (c *clothMesh) at(p int32, t float32) rl.Vector3 {
	if c.freedom[p] > 0 {
		return c.pos[p]
	}
	return c.targetAt(p, t)
}

func (c *clothMesh) targetAt(p int32, t float32) rl.Vector3 {
	return rl.Vector3Lerp(c.lastTarget[p], c.target[p], t)
}

// collide pushes x out of the capsules. A point right on a capsule's axis
// goes out towards target, where the pose puts it.
func collide(x, target rl.Vector3, colliders []worldCapsule) rl.Vector3 {
	for _, k := range colliders {
		closest := closestOnSegment(x, k.a, k.b)
		d := rl.Vector3Subtract(x, closest)
		dist := rl.Vector3Length(d)
		r := k.radius
		if dist >= r {
			continue
		}
		if dist < 1e-6 {
			d, dist = rl.Vector3Subtract(target, closest), rl.Vector3Distance(target, closest)
			if dist < 1e-6 {
				continue
			}
		}
		x = rl.Vector3Add(closest, rl.Vector3Scale(d, r/dist))
	}
	return x
}

func closestOnSegment(p, a, b rl.Vector3) rl.Vector3 {
	ab := rl.Vector3Subtract(b, a)
	l := rl.Vector3DotProduct(ab, ab)
	if l < 1e-12 {
		return a
	}
	t := rl.Vector3DotProduct(rl.Vector3Subtract(p, a), ab) / l
	return rl.Vector3Add(a, rl.Vector3Scale(ab, max(0, min(1, t))))
}

// simulate poses model for player p and advances cloth on it, drawn with
// matrix, by dt seconds. It skins every mesh itself rather than through
// raylib, which would only skin them again for the cloth to overwrite. It
// reports false, leaving the model alone, if the clip can't pose it.
func simulate(cloth *Cloth, model *rl.Model, p *AnimationPlayer, a *Animations, matrix rl.Matrix, dt float32) bool {
	meshes := model.GetMeshes()
	if cloth.state == nil {
		cloth.state = &clothState{meshes: map[int]*clothMesh{}}
		for i, cm := range cloth.Meshes {
			if i < 0 || i >= len(meshes) || meshes[i].BoneWeights == nil {
				continue
			}
			m := meshes[i]
			cloth.state.meshes[i] = newClothMesh(meshVertices(m), meshTriangles(m), cm.Freedom)
		}
	}
	s := cloth.state
	if !poseBones(s, model, p, a) {
		return false
	}
	bones := s.bones
	origin := rl.Vector3Transform(rl.Vector3{}, matrix)
	restart := rl.Vector3Distance(origin, s.origin) > clothTeleport
	s.origin = origin

	gravity := cloth.Gravity
	if gravity == (rl.Vector3{}) {
		gravity = rl.Vector3{Y: -9.8}
	}
	damping, stiffness := cloth.Damping, cloth.Stiffness
	if damping == 0 {
		damping = 0.03
	}
	if stiffness == 0 {
		stiffness = 0.02
	}
	thickness := cloth.Thickness
	if thickness == 0 {
		thickness = 0.005
	}
	colliders := placeColliders(cloth.Colliders, bones, matrix, thickness)

	s.spare += dt
	steps := int(s.spare / clothStep)
	s.spare -= float32(steps) * clothStep
	if steps > clothMaxSteps {
		steps, s.spare = clothMaxSteps, 0
	}
	turn := matrix
	turn.M12, turn.M13, turn.M14 = 0, 0, 0
	back := rl.MatrixInvert(matrix)

	if s.posed == nil {
		s.posed = make([]posedMesh, len(meshes))
	}
	for i, m := range meshes {
		if m.BoneWeights == nil || m.BoneIndices == nil || m.VertexCount == 0 {
			continue
		}
		n := int(m.VertexCount)
		ids, weights := unsafe.Slice(m.BoneIndices, 4*n), unsafe.Slice(m.BoneWeights, 4*n)
		pm := &s.posed[i]
		if len(pm.positions) != n {
			pm.positions = make([]rl.Vector3, n)
			if m.Normals != nil {
				pm.normals = make([]rl.Vector3, n)
			}
		}
		skin(pm.positions, meshVertices(m), ids, weights, bones)
		if m.Normals != nil {
			normals := unsafe.Slice((*rl.Vector3)(unsafe.Pointer(m.Normals)), n)
			for v := range normals {
				pm.normals[v] = rl.Vector3Normalize(skinNormal(normals[v], ids[4*v:4*v+4], weights[4*v:4*v+4], bones))
			}
		}

		if c := s.meshes[i]; c != nil {
			copy(c.lastTarget, c.target)
			for p, v := range c.first {
				c.target[p] = rl.Vector3Transform(pm.positions[v], matrix)
			}
			if pm.normals != nil {
				for _, p := range c.free {
					c.normal[p] = rl.Vector3Normalize(rl.Vector3Transform(pm.normals[c.first[p]], turn))
				}
			}
			if !c.started || restart {
				copy(c.pos, c.target)
				copy(c.prev, c.target)
				copy(c.lastTarget, c.target)
				c.started = true
			}
			for k := range steps {
				c.step(clothStep, float32(k+1)/float32(steps), gravity, 1-damping, stiffness, colliders)
			}
			// The moved particles back into the model's space, for drawing.
			for v, p := range c.particle {
				if c.freedom[p] > 0 {
					pm.positions[v] = rl.Vector3Transform(c.pos[p], back)
				}
			}
		}

		rl.UpdateMeshBuffer(m, 0, vectorBytes(pm.positions), 0)
		if pm.normals != nil {
			rl.UpdateMeshBuffer(m, vboNormalSlot, vectorBytes(pm.normals), 0)
		}
	}
	return true
}

// vboNormalSlot is where a mesh's normals are in raylib's Mesh.vboId.
const vboNormalSlot = 2

func vectorBytes(v []rl.Vector3) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(&v[0])), len(v)*int(unsafe.Sizeof(v[0])))
}

// posedMesh is a mesh's vertices as posed this frame, in model space.
type posedMesh struct {
	positions, normals []rl.Vector3
}

// poseBones works out the bone matrices for player p's pose, as raylib's
// UpdateModelAnimation(Ex) would (see bonePose), into s.bones.
func poseBones(s *clothState, model *rl.Model, p *AnimationPlayer, a *Animations) bool {
	bind := model.Skeleton.GetBindPose()
	if len(bind) == 0 {
		return false
	}
	if len(s.bones) != len(bind) {
		s.bones = make([]rl.Matrix, len(bind))
	}
	for i := range bind {
		pose, ok := bonePose(model, p, a, i)
		if !ok {
			return false
		}
		s.bones[i] = rl.MatrixMultiply(rl.MatrixInvert(poseMatrix(bind[i])), poseMatrix(pose))
	}
	return true
}

// placeColliders poses the capsules with their bones (bones being the
// model's bone matrices, which take the bind pose to the current one), puts
// them in world space and fattens them by the cloth's thickness.
func placeColliders(capsules []Capsule, bones []rl.Matrix, matrix rl.Matrix, thickness float32) []worldCapsule {
	scale := rl.Vector3Length(rl.Vector3{X: matrix.M0, Y: matrix.M1, Z: matrix.M2})
	out := make([]worldCapsule, 0, len(capsules))
	for _, k := range capsules {
		if k.BoneA < 0 || k.BoneB < 0 || k.BoneA >= len(bones) || k.BoneB >= len(bones) {
			continue
		}
		out = append(out, worldCapsule{
			a:      rl.Vector3Transform(rl.Vector3Transform(k.A, bones[k.BoneA]), matrix),
			b:      rl.Vector3Transform(rl.Vector3Transform(k.B, bones[k.BoneB]), matrix),
			radius: k.Radius*scale + thickness,
		})
	}
	return out
}

// meshVertices is m's bind positions.
func meshVertices(m rl.Mesh) []rl.Vector3 {
	if m.Vertices == nil {
		return nil
	}
	return unsafe.Slice((*rl.Vector3)(unsafe.Pointer(m.Vertices)), m.VertexCount)
}

// meshTriangles is m's triangles as vertex indices, three a triangle.
func meshTriangles(m rl.Mesh) []int32 {
	n := 3 * int(m.TriangleCount)
	out := make([]int32, n)
	if m.Indices == nil {
		for i := range out {
			out[i] = int32(i)
		}
		return out
	}
	for i, v := range unsafe.Slice(m.Indices, n) {
		out[i] = int32(v)
	}
	return out
}

// ClothFreedom is a helper for making ClothMesh.Freedom: for each vertex,
// rate times its distance along the mesh from the nearest pinned one, capped
// at most. pinned says which vertices are held; with none, nothing moves.
// Vertices at the same place count as one.
func ClothFreedom(vertices []rl.Vector3, triangles []int32, pinned []bool, rate, most float32) []float32 {
	c := newClothMesh(vertices, triangles, nil)
	n := len(c.first)
	// Every edge this time, pinned or not.
	adj := make([][]int32, n)
	for t := 0; t+2 < len(triangles); t += 3 {
		for k := range 3 {
			a, b := c.particle[triangles[t+k]], c.particle[triangles[t+(k+1)%3]]
			if a != b {
				adj[a] = append(adj[a], b)
				adj[b] = append(adj[b], a)
			}
		}
	}
	dist := make([]float32, n)
	for i := range dist {
		dist[i] = float32(math.Inf(1))
	}
	var queue distQueue
	for v, pin := range pinned {
		if p := c.particle[v]; pin && dist[p] != 0 {
			dist[p] = 0
			queue.push(p, 0)
		}
	}
	for queue.len() > 0 {
		p, d := queue.pop()
		if d > dist[p] {
			continue
		}
		for _, q := range adj[p] {
			nd := d + rl.Vector3Distance(vertices[c.first[p]], vertices[c.first[q]])
			if nd < dist[q] {
				dist[q] = nd
				queue.push(q, nd)
			}
		}
	}
	out := make([]float32, len(vertices))
	for v, p := range c.particle {
		if !math.IsInf(float64(dist[p]), 1) {
			out[v] = min(most, rate*dist[p])
		}
	}
	return out
}

// distQueue is a min-heap of particles by distance, for ClothFreedom.
type distQueue struct {
	p []int32
	d []float32
}

func (q *distQueue) len() int { return len(q.p) }

func (q *distQueue) push(p int32, d float32) {
	q.p, q.d = append(q.p, p), append(q.d, d)
	for i := len(q.p) - 1; i > 0; {
		parent := (i - 1) / 2
		if q.d[parent] <= q.d[i] {
			break
		}
		q.swap(i, parent)
		i = parent
	}
}

func (q *distQueue) pop() (int32, float32) {
	p, d := q.p[0], q.d[0]
	last := len(q.p) - 1
	q.swap(0, last)
	q.p, q.d = q.p[:last], q.d[:last]
	for i := 0; ; {
		l, r, small := 2*i+1, 2*i+2, i
		if l < last && q.d[l] < q.d[small] {
			small = l
		}
		if r < last && q.d[r] < q.d[small] {
			small = r
		}
		if small == i {
			break
		}
		q.swap(i, small)
		i = small
	}
	return p, d
}

func (q *distQueue) swap(i, j int) {
	q.p[i], q.p[j] = q.p[j], q.p[i]
	q.d[i], q.d[j] = q.d[j], q.d[i]
}
