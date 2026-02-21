package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
)

var (
	fps    = 60
	width  = 800
	height = 450
	title  = "Bumble Bee"
)

func main() {
	world := ecs.NewWorld()

	rl.InitWindow(int32(width), int32(height), title)

	defer rl.CloseWindow()
	rl.SetTargetFPS(int32(fps))

	systems := []illusion.System{
		illusion.NewPhysicsSystem(world),
		illusion.NewSpawnSystem(world),
		&illusion.Destroyer,
		NewTerrainSystem(world),
		NewCameraSystem(world),
		NewPlayerSystem(world),
	}

	for _, system := range systems {
		system.Startup()
	}

	defer (func() {
		for _, system := range systems {
			system.Shutdown()
		}
	})()

	camera := ecs.GetResource[rl.Camera3D](world)

	for !rl.WindowShouldClose() {
		rl.BeginDrawing()
		rl.BeginMode3D(*camera)

		rl.ClearBackground(rl.Black)

		dt := rl.GetFrameTime()
		for _, system := range systems {
			system.Update(dt)
		}

		rl.EndMode3D()

		rl.DrawFPS(0, 0)
		rl.EndDrawing()
	}
}
