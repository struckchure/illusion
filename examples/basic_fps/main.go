package main

import (
	"github.com/bbitechnologies/jolt-go/jolt"
	"github.com/struckchure/illusion"
)

func main() {
	app := illusion.NewApp()

	jolt.Init()
	defer jolt.Shutdown()

	app.
		AddSystem(illusion.Startup, physicsSystem).
		AddSystem(illusion.Startup, setupPlayer).
		AddSystem(illusion.Startup, setupCamera).
		AddSystem(illusion.Startup, setupFloor).
		AddSystem(illusion.Update, updateCamera).
		AddSystem(illusion.Update, inputSystem).
		AddSystem(illusion.Update, cameraFollowSystem).
		AddSystem(illusion.Update, movementSystem).
		AddSystem(illusion.Update, updatePlayerSystem).
		AddSystem(illusion.Update, drawFloorSystem)

	app.Run()
}
