package illusion

import (
	"github.com/bbitechnologies/jolt-go/jolt"
	"github.com/mlange-42/ark/ecs"
)

type PhysicsSystem struct {
	world *ecs.World

	joltPhysicsSystem *jolt.PhysicsSystem
	joltBodyInterface *jolt.BodyInterface
}

func (ps *PhysicsSystem) Startup() {
	jolt.Init()

	ps.joltPhysicsSystem = jolt.NewPhysicsSystem()
	ps.joltBodyInterface = ps.joltPhysicsSystem.GetBodyInterface()

	ecs.AddResource(ps.world, ps.joltPhysicsSystem)
	ecs.AddResource(ps.world, ps.joltBodyInterface)
}

func (ps *PhysicsSystem) Shutdown() {
	ps.joltPhysicsSystem.Destroy()
	jolt.Shutdown()
}

func (ps *PhysicsSystem) Update(dt float32) {
	ps.joltPhysicsSystem.Update(dt)
}

func NewPhysicsSystem(world *ecs.World) *PhysicsSystem {
	return &PhysicsSystem{world: world}
}
