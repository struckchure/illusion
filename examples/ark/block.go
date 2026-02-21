package main

import (
	"github.com/bbitechnologies/jolt-go/jolt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
)

type Block struct {
	Id    *jolt.BodyID
	World *ecs.World

	Position rl.Vector3
	Size     float32
	Color    rl.Color

	_shutdownFns []func()
}

func (b *Block) Startup() {
	bodyInterface := ecs.GetResource[jolt.BodyInterface](b.World)
	position := rl.Vector3Scale(b.Position, 0.5)

	halfExtents := jolt.Vec3{X: b.Size, Y: b.Size, Z: b.Size}.Mul(2)
	blockShape := jolt.CreateBox(halfExtents)
	illusion.Destroy(blockShape.Destroy)

	blockId := bodyInterface.CreateBody(
		blockShape,
		jolt.Vec3(position),
		jolt.MotionTypeDynamic,
		true,
	)

	b.Id = blockId
}

func (b *Block) Shutdown() {
	b.Id.Destroy()
}

func (b *Block) Update(dt float32) {
	bodyInterface := ecs.GetResource[jolt.BodyInterface](b.World)
	position := rl.Vector3(bodyInterface.GetPosition(b.Id))

	rl.DrawCube(position, b.Size, b.Size, b.Size, b.Color)
	rl.DrawCubeWires(
		position,
		b.Size, b.Size, b.Size,
		rl.White,
	)
}
