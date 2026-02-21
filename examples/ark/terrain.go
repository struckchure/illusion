package main

import (
	"github.com/bbitechnologies/jolt-go/jolt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
)

type Terrain struct {
	Id *jolt.BodyID
}

type TerrainSystem struct {
	world *ecs.World
}

var floorSize float32 = 100
var floorHeight float32 = 2

func (ts *TerrainSystem) Startup() {
	bodyInterface := ecs.GetResource[jolt.BodyInterface](ts.world)
	boxShape := jolt.CreateBox(jolt.Vec3{X: floorSize / 2, Y: floorHeight, Z: floorSize / 2})
	boxId := bodyInterface.CreateBody(
		boxShape,
		jolt.Vec3{X: 0, Y: -0.5, Z: 0},
		jolt.MotionTypeStatic,
		false,
	)

	terrain := Terrain{Id: boxId}
	ecs.AddResource(ts.world, &terrain)

	illusion.Spawn(
		ts.world,
		illusion.System(&Block{
			World:    ts.world,
			Position: rl.Vector3{X: 20, Y: 15, Z: -10},
			Size:     10,
			Color:    rl.Red,
		}),
	)

	illusion.Spawn(
		ts.world,
		illusion.System(&Block{
			World:    ts.world,
			Position: rl.Vector3{X: -20, Y: 15, Z: -10},
			Size:     5,
			Color:    rl.DarkGray,
		}),
	)
}

func (ts *TerrainSystem) Shutdown() {
	terrain := ecs.GetResource[Terrain](ts.world)
	terrain.Id.Destroy()
}

func (ts *TerrainSystem) Update(dt float32) {
	bodyInterface := ecs.GetResource[jolt.BodyInterface](ts.world)
	terrain := ecs.GetResource[Terrain](ts.world)
	position := rl.Vector3(bodyInterface.GetPosition(terrain.Id))

	rl.DrawGrid(int32(floorSize), 2)
	rl.DrawCubeWires(
		position,
		floorSize, floorHeight, floorSize,
		rl.White,
	)
	rl.DrawCube(
		position,
		floorSize, floorHeight, floorSize,
		rl.Blue,
	)
}

func NewTerrainSystem(world *ecs.World) *TerrainSystem {
	return &TerrainSystem{world: world}
}
