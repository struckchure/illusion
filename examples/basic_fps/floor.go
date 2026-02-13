package main

import (
	"github.com/bbitechnologies/jolt-go/jolt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/struckchure/illusion"
)

type Floor struct {
	ID jolt.BodyID
}

var (
	floorSize     = jolt.Vec3{X: 50, Y: 0.5, Z: 50}
	floorPosition = jolt.Vec3{X: 0, Y: -1.0, Z: 0}
)

func setupFloor(delta float32, app *illusion.App) {
	bi, _ := illusion.QuerySingle[*jolt.BodyInterface](app)

	floorID := bi.CreateBody(
		jolt.CreateBox(floorSize),
		floorPosition,
		jolt.MotionTypeStatic,
		true,
	)
	// defer floorID.Destroy()

	floor := illusion.NewComponent[Floor]()
	floor.Add(floor.Entity, Floor{ID: *floorID})

	app.AddComponent(floor)
}

func drawFloorSystem(delta float32, app *illusion.App) {
	rl.DrawCube(rl.Vector3(floorPosition), floorSize.X, floorSize.Y, floorSize.Z, rl.Gray)
	rl.DrawCubeWires(rl.Vector3(floorPosition), floorSize.X, floorSize.Y, floorSize.Z, rl.Black)
}
