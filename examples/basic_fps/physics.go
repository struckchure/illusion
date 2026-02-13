package main

import (
	"github.com/bbitechnologies/jolt-go/jolt"
	"github.com/struckchure/illusion"
)

func physicsSystem(_ float32, app *illusion.App) {
	ps := jolt.NewPhysicsSystem()
	bi := ps.GetBodyInterface()

	psComponent := illusion.NewComponent[*jolt.PhysicsSystem]()
	biComponent := illusion.NewComponent[*jolt.BodyInterface]()

	psComponent.Add(psComponent.Entity, ps)
	biComponent.Add(biComponent.Entity, bi)

	app.AddComponent(psComponent)
	app.AddComponent(biComponent)
}
