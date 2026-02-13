package main

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/struckchure/illusion"
)

const (
	JUMP_SPEED = 10

	ACCEL   = 10
	GRAVITY = 5
)

func inputSystem(delta float32, app *illusion.App) {
	playerControl := illusion.Query[Player](app)
	for entity, control := range playerControl.Data {
		velocity, ok := illusion.Query[Velocity](app).Get(entity)
		if !ok {
			fmt.Println("velocity not found")
			continue
		}
		position, ok := illusion.Query[Position](app).Get(entity)
		if !ok {
			fmt.Println("position not found")
			continue
		}

		// Reset velocity to 0 (so we stop when no key is pressed)
		velocity.X = 0
		velocity.Y = 0
		velocity.Z = 0

		// Reset Y position if on ground
		position.Y = rl.Lerp(position.Y, 0.5, GRAVITY*delta)

		// Check Keys
		// Note: In 3D, 'W' usually means moving "Forward" into the screen (-Z)
		if rl.IsKeyDown(rl.KeyW) || rl.IsKeyDown(rl.KeyUp) {
			velocity.Z = -control.Speed
		}
		if rl.IsKeyDown(rl.KeyS) || rl.IsKeyDown(rl.KeyDown) {
			velocity.Z = control.Speed
		}
		if rl.IsKeyDown(rl.KeyA) || rl.IsKeyDown(rl.KeyLeft) {
			velocity.X = -control.Speed
		}
		if rl.IsKeyDown(rl.KeyD) || rl.IsKeyDown(rl.KeyRight) {
			velocity.X = control.Speed
		}

		if rl.IsKeyPressed(rl.KeySpace) {
			velocity.Y = JUMP_SPEED
		}
	}
}
