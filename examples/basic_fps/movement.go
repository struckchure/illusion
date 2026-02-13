package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/struckchure/illusion"
)

func movementSystem(delta float32, app *illusion.App) {
	_, _, position := illusion.QuerySingleWith[Player, Position](app)
	_, _, velocity := illusion.QuerySingleWith[Player, Velocity](app)

	nextPosition := rl.Vector3Add(
		rl.Vector3(*position),
		rl.Vector3(*velocity),
	)

	// Euler Integration: Pos = Pos + (Vel * dt)
	*position = Position(rl.Vector3Lerp(rl.Vector3(*position), nextPosition, 2.0*delta))
}
