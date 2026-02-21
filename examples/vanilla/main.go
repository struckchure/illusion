package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

var (
	fps    = 60
	width  = 800
	height = 450
	title  = "Bumble Bee"

	cameraDistance float32 = 10.0
	cameraHeight   float32 = 5.0

	modelFile   = "./examples/assets/bee.obj"
	textureFile = "./examples/assets/palette.png"

	playerSize float32 = 1.0
)

const (
	WALK_SPEED = 2.0
	JUMP_SPEED = 6.0

	ACC_RATE  = 0.2
	DECC_RATE = 0.1

	SMOOTH_FACTOR = 0.1
)

func main() {
	position := rl.NewVector3(0, 0, 0)
	velocity := rl.NewVector3(0, 0, 0)

	rl.InitWindow(int32(width), int32(height), title)

	defer rl.CloseWindow()
	rl.SetTargetFPS(int32(fps))

	camera := rl.Camera3D{
		Position:   rl.NewVector3(0, cameraHeight, cameraDistance), // Camera location
		Target:     rl.NewVector3(0.0, 0.0, 0.0),                   // Looking at center
		Up:         rl.NewVector3(0.0, 1.0, 0.0),                   // Camera "up" vector
		Fovy:       45.0,                                           // Field of view
		Projection: rl.CameraPerspective,
	}

	model := rl.LoadModel(modelFile)
	defer rl.UnloadModel(model)

	texture := rl.LoadTexture(textureFile)
	defer rl.UnloadTexture(texture)

	for !rl.WindowShouldClose() {
		velocity.X = 0
		velocity.Y = 0
		velocity.Z = 0

		// Reset Y position if on ground
		position.Y = rl.Lerp(position.Y, 1, DECC_RATE)

		rl.BeginDrawing()
		rl.BeginMode3D(camera)

		rl.ClearBackground(rl.DarkGray)

		// Check Keys
		// Note: In 3D, 'W' usually means moving "Forward" into the screen (-Z)
		if rl.IsKeyDown(rl.KeyW) || rl.IsKeyDown(rl.KeyUp) {
			velocity.Z = -WALK_SPEED
		}
		if rl.IsKeyDown(rl.KeyS) || rl.IsKeyDown(rl.KeyDown) {
			velocity.Z = WALK_SPEED
		}
		if rl.IsKeyDown(rl.KeyA) || rl.IsKeyDown(rl.KeyLeft) {
			velocity.X = -WALK_SPEED
		}
		if rl.IsKeyDown(rl.KeyD) || rl.IsKeyDown(rl.KeyRight) {
			velocity.X = WALK_SPEED
		}

		if rl.IsKeyPressed(rl.KeySpace) {
			velocity.Y = JUMP_SPEED
		}

		nextPosition := rl.Vector3Add(position, velocity)

		// Euler Integration: Pos = Pos + (Vel * dt)
		nextPosition = rl.Vector3Lerp(position, nextPosition, ACC_RATE)

		position.X = nextPosition.X
		position.Y = nextPosition.Y
		position.Z = nextPosition.Z

		camera.Target = rl.Vector3Lerp(camera.Target, position, SMOOTH_FACTOR)
		idealPos := rl.Vector3Add(position, rl.NewVector3(0.0, cameraHeight, cameraDistance))
		camera.Position = rl.Vector3Lerp(camera.Position, idealPos, SMOOTH_FACTOR)

		rl.SetMaterialTexture(model.Materials, rl.MapDiffuse, texture)
		rl.DrawModel(model, position, playerSize, rl.White)

		rl.DrawGrid(50, 2)
		rl.EndMode3D()
		rl.EndDrawing()
	}
}
