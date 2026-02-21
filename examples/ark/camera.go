package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
)

var (
	cameraHeight   float32 = 5.0
	cameraDistance float32 = 20.0
)

type CameraSystem struct {
	world        *ecs.World
	playerFilter *ecs.Filter1[Player]
}

func (cs *CameraSystem) Startup() {
	cs.playerFilter = ecs.NewFilter1[Player](cs.world)

	camera := rl.Camera3D{
		Position:   rl.NewVector3(0.0, cameraHeight, cameraDistance),
		Target:     rl.NewVector3(0.0, 0.0, 0.0),
		Up:         rl.NewVector3(0.0, 2.0, 0.0),
		Fovy:       45.0,
		Projection: rl.CameraPerspective,
	}

	ecs.AddResource(cs.world, &camera)
}

func (cs *CameraSystem) Shutdown() {}

func (cs *CameraSystem) Update(dt float32) {
	camera := ecs.GetResource[rl.Camera3D](cs.world)

	query := cs.playerFilter.Query()
	for query.Next() {
		playerPosition := query.Get().GetPosition()

		camera.Target = rl.Vector3Lerp(camera.Target, rl.Vector3(playerPosition), SMOOTH_FACTOR)
		idealPos := rl.Vector3Add(rl.Vector3(playerPosition), rl.NewVector3(0.0, cameraHeight, cameraDistance))
		camera.Position = rl.Vector3Lerp(camera.Position, idealPos, SMOOTH_FACTOR)
	}
}

func NewCameraSystem(world *ecs.World) illusion.System {
	return &CameraSystem{world: world}
}
