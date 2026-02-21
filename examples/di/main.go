package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/samber/do"
	"github.com/struckchure/illusion"
)

var (
	fps    = 60
	width  = 800
	height = 450
	title  = "Bumble Bee"
)

func main() {
	do.Provide(nil, NewPlayer)
	do.Provide(nil, NewCamera)

	rl.InitWindow(int32(width), int32(height), title)

	defer rl.CloseWindow()
	rl.SetTargetFPS(int32(fps))

	player := do.MustInvoke[*Player](nil)
	camera := do.MustInvoke[*Camera](nil)

	systems := []illusion.System{player, camera}

	for _, system := range systems {
		system.Startup()
	}

	defer (func() {
		for _, system := range systems {
			system.Shutdown()
		}
	})()

	for !rl.WindowShouldClose() {
		rl.BeginDrawing()
		rl.BeginMode3D(*camera.Camera3D)

		rl.ClearBackground(rl.DarkGray)

		dt := rl.GetFrameTime()

		for _, system := range systems {
			system.Update(dt)
		}

		rl.DrawFPS(0, 0)
		rl.DrawGrid(50, 2)
		rl.EndMode3D()
		rl.EndDrawing()
	}
}
