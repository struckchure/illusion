package physics

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/internal/jolt"
	"github.com/struckchure/illusion/transform"
)

// Sets in the FixedPostUpdate schedule, in the order they run. Game logic
// that drives physics (forces, character Walk) belongs in FixedUpdate.
const (
	// Prepare creates and removes bodies and pushes Transform and Velocity
	// changes into the simulation.
	Prepare illusion.SystemSet = "physics.Prepare"
	// Step moves characters and advances the simulation.
	Step illusion.SystemSet = "physics.Step"
	// Writeback copies results into Transform, Velocity and
	// CharacterController, and sends collision events.
	Writeback illusion.SystemSet = "physics.Writeback"
)

// Settings is a resource that controls the simulation.
type Settings struct {
	Gravity rl.Vector3
	// Substeps splits each fixed step for more stable stacking. Defaults to 1.
	Substeps int
	// Paused stops the simulation.
	Paused bool
}

// Plugin adds Jolt physics.
type Plugin struct {
	// MaxBodies is the most bodies the world can hold. Defaults to 10240.
	MaxBodies int
}

// Build implements [illusion.Plugin].
func (p Plugin) Build(app *illusion.App) {
	maxBodies := p.MaxBodies
	if maxBodies <= 0 {
		maxBodies = 10240
	}
	w := &world{
		jolt:     jolt.NewWorld(maxBodies),
		entities: map[jolt.BodyID]ecs.Entity{},
		carry:    map[ecs.Entity]Velocity{},
	}
	app.InsertResource(illusion.R(w))
	app.InitResource(illusion.R(&Settings{Gravity: rl.Vector3{Y: -9.81}, Substeps: 1}))
	illusion.AddEvent[CollisionStarted](app)
	illusion.AddEvent[CollisionEnded](app)
	w.observe(app.World)

	app.ConfigureSets(illusion.FixedPostUpdate, Step.After(Prepare), Writeback.After(Step))
	app.AddSystems(illusion.FixedPostUpdate,
		illusion.Chain(
			illusion.Fn4(rebuildChanged).Named("physics.rebuildChanged"),
			illusion.Fn4(removeStaleBodies).Named("physics.removeStale"),
			illusion.Fn4(createBodies).Named("physics.createBodies"),
			illusion.Fn2(rebuildCharacters).Named("physics.rebuildCharacters"),
			illusion.Fn3(createCharacters).Named("physics.createCharacters"),
			illusion.Fn4(pushChanges).Named("physics.pushChanges"),
		).InSet(Prepare),
		illusion.Chain(
			illusion.Fn3(moveCharacters).Named("physics.moveCharacters"),
			illusion.Fn3(step).Named("physics.step"),
		).InSet(Step),
		illusion.Group(
			illusion.Fn4(pullBodies).Named("physics.pullBodies"),
			illusion.Fn2(pullCharacters).Named("physics.pullCharacters"),
			illusion.Fn3(sendCollisions).Named("physics.sendCollisions"),
		).InSet(Writeback),
	)
}

// Cleanup implements the optional plugin cleanup hook.
func (Plugin) Cleanup(app *illusion.App) {
	w := ecs.GetResource[world](app.World)
	if w == nil || w.jolt == nil {
		return
	}
	// Characters own Jolt objects that must go before the world does.
	query := ecs.NewFilter1[character](app.World).Query()
	for query.Next() {
		query.Get().c.Close()
	}
	w.jolt.Close()
	w.jolt = nil
}

// world is the resource holding the Jolt world and the body→entity map.
type world struct {
	jolt     *jolt.World
	entities map[jolt.BodyID]ecs.Entity
	contacts []jolt.Contact
	// removed holds destroyed bodies whose entity mapping is kept until the
	// next sendCollisions, so the contacts they ended still name the entity.
	removed []jolt.BodyID
	// carry keeps the velocity of bodies being rebuilt without a Velocity
	// component, so they keep moving.
	carry   map[ecs.Entity]Velocity
	gravity rl.Vector3
	gravSet bool
}

// body links an entity to its Jolt body.
type body struct {
	id jolt.BodyID
	// The settings the body was built from; a change rebuilds it.
	config bodyConfig
	// What the plugin last wrote (or read), to spot user changes.
	lastTransform transform.Transform
	lastVelocity  Velocity
}

// bodyConfig is everything a body is built from, besides its pose and
// velocity. It is comparable, so a change is a single != away.
type bodyConfig struct {
	rigidBody                   RigidBody
	collider                    colliderKey
	sensor, lock, continuous    bool
	material                    Material
	mass                        Mass
	gravityScale                GravityScale
	damping                     Damping
	hasMaterial, hasMass        bool
	hasGravityScale, hasDamping bool
}

// colliderKey identifies a Collider. Point and index slices are compared by
// identity: replacing the Collider with a newly built one counts as a change.
type colliderKey struct {
	kind      shapeKind
	size      [3]float32
	offset    rl.Vector3
	hasOffset bool
	points    *[3]float32
	nPoints   int
	indices   *uint32
	nIndices  int
}

func keyOf(c *Collider) colliderKey {
	k := colliderKey{kind: c.kind, size: c.size, offset: c.offset, hasOffset: c.hasOffset,
		nPoints: len(c.points), nIndices: len(c.indices)}
	if len(c.points) > 0 {
		k.points = &c.points[0]
	}
	if len(c.indices) > 0 {
		k.indices = &c.indices[0]
	}
	return k
}

// character links an entity to its Jolt character.
type character struct {
	c    *jolt.Character
	body jolt.BodyID
	// The controller settings the character was built from; a change
	// rebuilds it.
	config characterConfig
}

type characterConfig struct {
	radius, height, maxSlope float32
}

func characterConfigOf(cc *CharacterController) characterConfig {
	return characterConfig{radius: cc.Radius, height: cc.Height, maxSlope: cc.MaxSlope}
}

// observe destroys Jolt objects when their entity or component goes away.
func (w *world) observe(ecsWorld *ecs.World) {
	dropBody := func(_ ecs.Entity, b *body) {
		if w.jolt != nil {
			w.jolt.DestroyBody(b.id)
		}
		w.removed = append(w.removed, b.id)
	}
	dropCharacter := func(_ ecs.Entity, c *character) {
		if w.jolt != nil {
			c.c.Close()
		}
		w.removed = append(w.removed, c.body)
	}
	ecs.Observe1[body](ecs.OnRemoveEntity).Do(dropBody).Register(ecsWorld)
	ecs.Observe1[body](ecs.OnRemoveComponents).Do(dropBody).Register(ecsWorld)
	ecs.Observe1[character](ecs.OnRemoveEntity).Do(dropCharacter).Register(ecsWorld)
	ecs.Observe1[character](ecs.OnRemoveComponents).Do(dropCharacter).Register(ecsWorld)
}

// removeStaleBodies drops bodies whose RigidBody or Collider was removed, and
// characters whose controller was.
func removeStaleBodies(
	noRigidBody *illusion.Query0Where[illusion.And[illusion.With[body], illusion.Without[RigidBody]]],
	noCollider *illusion.Query0Where[illusion.And[illusion.With[body], illusion.Without[Collider]]],
	noController *illusion.Query0Where[illusion.And[illusion.With[character], illusion.Without[CharacterController]]],
	cmd *illusion.Commands,
) {
	noRigidBody.Each(func(e ecs.Entity) { cmd.Entity(e).Remove(ecs.C[body]()) })
	noCollider.Each(func(e ecs.Entity) { cmd.Entity(e).Remove(ecs.C[body]()) })
	noController.Each(func(e ecs.Entity) { cmd.Entity(e).Remove(ecs.C[character]()) })
}

// optional reads the optional per-body components.
type optional struct {
	world        *ecs.World
	sensor       *ecs.Map[Sensor]
	material     *ecs.Map[Material]
	mass         *ecs.Map[Mass]
	gravityScale *ecs.Map[GravityScale]
	damping      *ecs.Map[Damping]
	lock         *ecs.Map[LockRotation]
	continuous   *ecs.Map[ContinuousCollision]
	velocity     *ecs.Map[Velocity]
}

func (o *optional) InitParam(w *ecs.World) {
	o.world = w
	o.sensor = ecs.NewMap[Sensor](w)
	o.material = ecs.NewMap[Material](w)
	o.mass = ecs.NewMap[Mass](w)
	o.gravityScale = ecs.NewMap[GravityScale](w)
	o.damping = ecs.NewMap[Damping](w)
	o.lock = ecs.NewMap[LockRotation](w)
	o.continuous = ecs.NewMap[ContinuousCollision](w)
	o.velocity = ecs.NewMap[Velocity](w)
}

// config reads the settings e's body should be built from.
func (o *optional) config(e ecs.Entity, rb *RigidBody, col *Collider) bodyConfig {
	c := bodyConfig{
		rigidBody:  *rb,
		collider:   keyOf(col),
		sensor:     o.sensor.Has(e),
		lock:       o.lock.Has(e),
		continuous: o.continuous.Has(e),
	}
	if m := o.material.Get(e); m != nil {
		c.material, c.hasMaterial = *m, true
	}
	if m := o.mass.Get(e); m != nil {
		c.mass, c.hasMass = *m, true
	}
	if g := o.gravityScale.Get(e); g != nil {
		c.gravityScale, c.hasGravityScale = *g, true
	}
	if d := o.damping.Get(e); d != nil {
		c.damping, c.hasDamping = *d, true
	}
	return c
}

// rebuildChanged drops bodies whose settings changed since they were built
// (say, RigidBody switched to Static or a new Collider was inserted), so
// createBodies builds them again this step.
func rebuildChanged(
	q *illusion.Query3[body, RigidBody, Collider],
	opt *optional,
	res *illusion.Res[world],
	cmd *illusion.Commands,
) {
	w := res.Get()
	query := q.Iter()
	for query.Next() {
		b, rb, col := query.Get()
		e := query.Entity()
		if opt.config(e, rb, col) == b.config {
			continue
		}
		if !opt.velocity.Has(e) {
			lin, ang := w.jolt.Velocity(b.id)
			w.carry[e] = Velocity{Linear: vec(lin), Angular: vec(ang)}
		}
		cmd.Entity(e).Remove(ecs.C[body]())
	}
}

func createBodies(
	q *illusion.Query3Where[RigidBody, Collider, transform.Transform, illusion.Without[body]],
	opt *optional,
	res *illusion.Res[world],
	cmd *illusion.Commands,
) {
	w := res.Get()
	query := q.Iter()
	for query.Next() {
		e := query.Entity()
		rb, col, tr := query.Get()
		cfg := opt.config(e, rb, col)
		shape, err := buildShape(col)
		if err != nil {
			panic(fmt.Sprintf("physics: entity %v: %v", e, err))
		}

		s := jolt.DefaultBodySettings(shape)
		s.Transform = joltTransform(*tr)
		switch cfg.rigidBody {
		case Static:
			s.Motion = jolt.Static
		case Kinematic:
			s.Motion = jolt.Kinematic
		default:
			s.Motion = jolt.Dynamic
		}
		s.Sensor = cfg.sensor
		s.Continuous = cfg.continuous
		if cfg.lock {
			s.AllowedDOFs = jolt.TranslationX | jolt.TranslationY | jolt.TranslationZ
		}
		if cfg.hasMaterial {
			s.Friction, s.Restitution = cfg.material.Friction, cfg.material.Restitution
		}
		if cfg.hasMass {
			s.Mass = float32(cfg.mass)
		}
		if cfg.hasGravityScale {
			s.GravityFactor = float32(cfg.gravityScale)
		}
		if cfg.hasDamping {
			s.LinearDamping, s.AngularDamping = cfg.damping.Linear, cfg.damping.Angular
		}
		vel, carried := w.carry[e]
		delete(w.carry, e)
		if v := opt.velocity.Get(e); v != nil {
			vel, carried = *v, true
		}
		if carried {
			s.LinearVelocity, s.AngularVelocity = v3(vel.Linear), v3(vel.Angular)
		}

		id := w.jolt.CreateBody(s)
		shape.Release()
		w.entities[id] = e
		cmd.Entity(e).Insert(illusion.C(body{id: id, config: cfg, lastTransform: *tr, lastVelocity: vel}))
	}
}

func buildShape(c *Collider) (*jolt.Shape, error) {
	var s *jolt.Shape
	var err error
	switch c.kind {
	case shapeBox:
		s, err = jolt.NewBox(c.size)
	case shapeSphere:
		s, err = jolt.NewSphere(c.size[0])
	case shapeCapsule:
		s, err = jolt.NewCapsule(c.size[1], c.size[0])
	case shapeCylinder:
		s, err = jolt.NewCylinder(c.size[1], c.size[0])
	case shapeConvexHull:
		s, err = jolt.NewConvexHull(c.points)
	case shapeMesh:
		s, err = jolt.NewMesh(c.points, c.indices)
	}
	if err != nil || !c.hasOffset {
		return s, err
	}
	moved, err := s.Offset(v3(c.offset))
	s.Release()
	return moved, err
}

// rebuildCharacters drops characters whose controller size or slope changed,
// so createCharacters builds them again at their current position.
func rebuildCharacters(q *illusion.Query2[CharacterController, character], cmd *illusion.Commands) {
	q.Each(func(e ecs.Entity, cc *CharacterController, ch *character) {
		if characterConfigOf(cc) != ch.config {
			cmd.Entity(e).Remove(ecs.C[character]())
		}
	})
}

func createCharacters(
	q *illusion.Query2Where[CharacterController, transform.Transform, illusion.Without[character]],
	res *illusion.Res[world],
	cmd *illusion.Commands,
) {
	w := res.Get()
	query := q.Iter()
	for query.Next() {
		e := query.Entity()
		cc, tr := query.Get()
		radius := cc.Radius
		if radius <= 0 {
			radius = 0.3
		}
		height := max(cc.Height, 2*radius+0.01)
		slope := cc.MaxSlope
		if slope <= 0 {
			slope = math.Pi / 4
		}
		shape, err := jolt.NewCapsule(height/2-radius, radius)
		if err != nil {
			panic(fmt.Sprintf("physics: entity %v: %v", e, err))
		}
		c := w.jolt.NewCharacter(shape, v3(tr.Translation), slope, radius)
		shape.Release()
		inner := c.InnerBody()
		w.entities[inner] = e
		cmd.Entity(e).Insert(illusion.C(character{c: c, body: inner, config: characterConfigOf(cc)}))
	}
}

// pushChanges sends Transform and Velocity edits made by game code to Jolt,
// and drives kinematic bodies toward their Transform.
func pushChanges(
	q *illusion.Query3[body, RigidBody, transform.Transform],
	velocities *illusion.Query1[Velocity],
	res *illusion.Res[world],
	t *illusion.Res[illusion.Time],
) {
	w := res.Get()
	dt := t.Get().DeltaSecs()
	query := q.Iter()
	for query.Next() {
		b, rb, tr := query.Get()
		moved := tr.Translation != b.lastTransform.Translation || tr.Rotation != b.lastTransform.Rotation
		switch {
		case *rb == Kinematic:
			w.jolt.MoveKinematic(b.id, joltTransform(*tr), dt)
		case moved:
			w.jolt.SetTransform(b.id, joltTransform(*tr), true)
		}
		b.lastTransform = *tr

		if v, ok := velocities.Get(query.Entity()); ok && *rb == Dynamic && *v != b.lastVelocity {
			w.jolt.SetVelocity(b.id, v3(v.Linear), v3(v.Angular))
			b.lastVelocity = *v
		}
	}
}

func moveCharacters(
	q *illusion.Query3[CharacterController, character, transform.Transform],
	settings *illusion.Res[Settings],
	t *illusion.Res[illusion.Time],
) {
	dt := t.Get().DeltaSecs()
	g := settings.Get().Gravity
	if settings.Get().Paused || dt <= 0 {
		return
	}
	query := q.Iter()
	for query.Next() {
		cc, ch, tr := query.Get()
		// A Transform moved by game code teleports the character.
		if p := ch.c.Position(); vec(p) != tr.Translation {
			ch.c.SetPosition(v3(tr.Translation))
		}
		v := cc.Velocity
		v.X, v.Z = cc.Walk.X, cc.Walk.Z
		if ch.c.Supported() && v.Y <= 0 {
			v.Y = ch.c.GroundVelocity()[1]
		} else {
			v = rl.Vector3Add(v, rl.Vector3Scale(g, dt))
		}
		ch.c.SetVelocity(v3(v))
		ch.c.Update(dt, cc.StepHeight)
	}
}

func step(res *illusion.Res[world], settings *illusion.Res[Settings], t *illusion.Res[illusion.Time]) {
	w, s := res.Get(), settings.Get()
	if s.Paused {
		return
	}
	if !w.gravSet || w.gravity != s.Gravity {
		w.jolt.SetGravity(v3(s.Gravity))
		w.gravity, w.gravSet = s.Gravity, true
	}
	if err := w.jolt.Step(t.Get().DeltaSecs(), max(s.Substeps, 1)); err != nil {
		panic(err)
	}
}

func pullBodies(
	q *illusion.Query3[body, RigidBody, transform.Transform],
	velocities *illusion.Query1[Velocity],
	res *illusion.Res[world],
	settings *illusion.Res[Settings],
) {
	if settings.Get().Paused {
		return
	}
	w := res.Get()
	query := q.Iter()
	for query.Next() {
		b, rb, tr := query.Get()
		if *rb != Dynamic {
			continue
		}
		jt := w.jolt.Transform(b.id)
		tr.Translation = vec(jt.Position)
		tr.Rotation = rl.Quaternion{X: jt.Rotation[0], Y: jt.Rotation[1], Z: jt.Rotation[2], W: jt.Rotation[3]}
		b.lastTransform = *tr
		if v, ok := velocities.Get(query.Entity()); ok {
			lin, ang := w.jolt.Velocity(b.id)
			*v = Velocity{Linear: vec(lin), Angular: vec(ang)}
			b.lastVelocity = *v
		}
	}
}

func pullCharacters(q *illusion.Query3[CharacterController, character, transform.Transform], settings *illusion.Res[Settings]) {
	if settings.Get().Paused {
		return
	}
	query := q.Iter()
	for query.Next() {
		cc, ch, tr := query.Get()
		tr.Translation = vec(ch.c.Position())
		cc.Velocity = vec(ch.c.Velocity())
		cc.Grounded = ch.c.Supported()
		cc.GroundNormal = vec(ch.c.GroundNormal())
	}
}

func sendCollisions(
	res *illusion.Res[world],
	started *illusion.EventWriter[CollisionStarted],
	ended *illusion.EventWriter[CollisionEnded],
) {
	w := res.Get()
	w.contacts = w.jolt.DrainContacts(w.contacts[:0])
	for _, c := range w.contacts {
		a, okA := w.entities[c.Body1]
		b, okB := w.entities[c.Body2]
		if !okA || !okB {
			continue // not a body the plugin made
		}
		if c.Began {
			started.Send(CollisionStarted{A: a, B: b, Point: vec(c.Point), Normal: vec(c.Normal)})
		} else {
			ended.Send(CollisionEnded{A: a, B: b})
		}
	}
	// Destroyed bodies have reported their last contacts; forget them.
	for _, id := range w.removed {
		delete(w.entities, id)
	}
	w.removed = w.removed[:0]
}

func joltTransform(t transform.Transform) jolt.Transform {
	return jolt.Transform{
		Position: v3(t.Translation),
		Rotation: [4]float32{t.Rotation.X, t.Rotation.Y, t.Rotation.Z, t.Rotation.W},
	}
}
