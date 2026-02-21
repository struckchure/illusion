package illusion

import (
	"fmt"
	"sync"

	"github.com/mlange-42/ark/ecs"
)

type SpawnSystem struct {
	world *ecs.World

	systems []System
}

func (ss *SpawnSystem) Startup() {
	ecs.Observe1[System](ecs.OnCreateEntity).
		Do(func(entity ecs.Entity, system *System) {
			fmt.Println("Adding System to Entity: ", entity.ID())
			ss.systems = append(ss.systems, *system)
			ss.systems[len(ss.systems)-1].Startup()
		}).
		Register(ss.world)
}

func (ss *SpawnSystem) Shutdown() {
	for _, system := range ss.systems {
		system.Shutdown()
	}
}

func (ss *SpawnSystem) Update(dt float32) {
	for _, system := range ss.systems {
		system.Update(dt)
	}
}

func NewSpawnSystem(world *ecs.World) *SpawnSystem {
	return &SpawnSystem{world: world, systems: []System{}}
}

var (
	builder     *ecs.Map1[System]
	onceBuilder sync.Once
)

func Spawn(world *ecs.World, system System, rel ...ecs.Relation) ecs.Entity {
	onceBuilder.Do(func() {
		builder = ecs.NewMap1[System](world)
	})

	return builder.NewEntity(&system, rel...)
}
