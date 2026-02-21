package main

import (
	"math"

	"github.com/bbitechnologies/jolt-go/jolt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
)

const (
	WALK_SPEED = 5.0
	JUMP_SPEED = 7.0

	GRAVITY = -9.81

	ACC_RATE  = 0.2
	DECC_RATE = 0.1

	SMOOTH_FACTOR = 0.1

	PLAYER_SIZE float32 = 1.0
)

type PlayerAsset struct {
	model   rl.Model
	texture rl.Texture2D
}

type Player struct {
	*jolt.CharacterVirtual
}

type PlayerSystem struct {
	world  *ecs.World
	mapper *ecs.Map1[Player]
	filter *ecs.Filter1[Player]
}

func (ps *PlayerSystem) Startup() {
	ps.mapper = ecs.NewMap1[Player](ps.world)
	ps.filter = ecs.NewFilter1[Player](ps.world)

	model := rl.LoadModel("./examples/assets/bee.obj")
	texture := rl.LoadTexture("./examples/assets/palette.png")

	physicsSystem := ecs.GetResource[jolt.PhysicsSystem](ps.world)

	capsule := jolt.CreateCapsule(PLAYER_SIZE/4, PLAYER_SIZE)
	characterSettings := jolt.NewCharacterVirtualSettings(capsule)
	character := Player{
		physicsSystem.CreateCharacterVirtual(
			characterSettings,
			jolt.Vec3{X: 0, Y: 20, Z: 0},
		),
	}

	ecs.AddResource(ps.world, &character)
	ecs.AddResource(ps.world, &PlayerAsset{model: model, texture: texture})

	ps.mapper.NewEntity(&character)
}

func (ps *PlayerSystem) Shutdown() {
	playerAssets := ecs.GetResource[PlayerAsset](ps.world)

	rl.UnloadModel(playerAssets.model)
	rl.UnloadTexture(playerAssets.texture)
}

func (ps *PlayerSystem) input() rl.Vector3 {
	direction := rl.Vector3{X: 0, Y: 0, Z: 0}

	direction.X = 0
	direction.Y = 0
	direction.Z = 0

	// Check Keys
	// Note: In 3D, 'W' usually means moving "Forward" into the screen (-Z)
	if rl.IsKeyDown(rl.KeyW) || rl.IsKeyDown(rl.KeyUp) {
		direction.Z = -WALK_SPEED
	}
	if rl.IsKeyDown(rl.KeyS) || rl.IsKeyDown(rl.KeyDown) {
		direction.Z = WALK_SPEED
	}
	if rl.IsKeyDown(rl.KeyA) || rl.IsKeyDown(rl.KeyLeft) {
		direction.X = -WALK_SPEED
	}
	if rl.IsKeyDown(rl.KeyD) || rl.IsKeyDown(rl.KeyRight) {
		direction.X = WALK_SPEED
	}

	return direction
}

func (ps *PlayerSystem) normalize(direction rl.Vector3) float32 {
	return float32(math.Sqrt(float64(direction.X*direction.X + direction.Z*direction.Z)))
}

func (ps *PlayerSystem) Update(dt float32) {
	playerAssets := ecs.GetResource[PlayerAsset](ps.world)

	query := ps.filter.Query()
	for query.Next() {
		player := query.Get()

		direction := ps.input()

		// Normalize horizontal movement (prevent faster diagonal movement)
		magnitude := ps.normalize(direction)
		if magnitude > 0 {
			direction.X /= magnitude
			direction.Z /= magnitude
		}

		var velocity jolt.Vec3
		if player.IsSupported() {
			// On ground, read only horizontal velocity
			velocity = player.GetGroundVelocity()
		} else {
			// In air, read full velocity
			velocity = player.GetLinearVelocity()
		}

		// Apply move speed to horizontal velocity
		velocity.X = direction.X * WALK_SPEED
		velocity.Z = direction.Z * WALK_SPEED

		// Handle jumping (only when grounded)
		if rl.IsKeyPressed(rl.KeySpace) && player.IsSupported() {
			velocity.Y = JUMP_SPEED
		}

		// Apply gravity
		velocity.Y += GRAVITY * dt

		// Set desired velocity and call extended update to resolve movement
		player.SetLinearVelocity(velocity)
		gravity := jolt.Vec3{X: 0, Y: GRAVITY, Z: 0}
		player.ExtendedUpdate(dt, gravity)
		position := player.GetPosition()

		rl.SetMaterialTexture(playerAssets.model.Materials, rl.MapDiffuse, playerAssets.texture)
		rl.DrawModel(playerAssets.model, rl.Vector3(position), PLAYER_SIZE, rl.White)
	}
}

func NewPlayerSystem(world *ecs.World) illusion.System {
	return &PlayerSystem{world: world}
}
