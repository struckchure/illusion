package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/struckchure/illusion"
)

const cameraDistance = 10.0
const cameraHeight = 5.0

func setupCamera(delta float32, app *illusion.App) {
	_, player := illusion.QuerySingle[Player](app)

	camera := illusion.NewComponent[rl.Camera3D]()
	camera.Add(*player, rl.Camera3D{
		Position:   rl.NewVector3(cameraDistance, cameraHeight, cameraDistance), // Camera location
		Target:     rl.NewVector3(0.0, 0.0, 0.0),                                // Looking at center
		Up:         rl.NewVector3(0.0, 1.0, 0.0),                                // Camera "up" vector
		Fovy:       45.0,                                                        // Field of view
		Projection: rl.CameraPerspective,
	})
	app.AddComponent(camera)
}

func updateCamera(_ float32, app *illusion.App) {
	camera, _ := illusion.QuerySingle[rl.Camera3D](app)

	rl.BeginMode3D(camera)
}

func cameraFollowSystem(dt float32, app *illusion.App) {
	_, _, playerPosition := illusion.QuerySingleWith[Player, Position](app)
	camera, _ := illusion.QuerySingle[rl.Camera3D](app)

	// 1. Target: Smoothly look at player
	// Lerp factor 0.1 means "move 10% of the way there every frame"
	camera.Target = rl.Vector3Lerp(camera.Target, rl.Vector3(*playerPosition), 5.0*dt)

	// 2. Position: Calculate ideal spot
	idealPos := rl.Vector3Add(rl.Vector3(*playerPosition), rl.NewVector3(0.0, cameraHeight, cameraDistance))

	// Smoothly move camera towards ideal spot
	camera.Position = rl.Vector3Lerp(camera.Position, idealPos, 5.0*dt)
}
