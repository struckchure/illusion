package main

import (
	"fmt"

	"github.com/bbitechnologies/jolt-go/jolt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/struckchure/illusion"
)

type Position rl.Vector3
type Velocity rl.Vector3

type Player struct {
	ID      *jolt.BodyID
	Speed   float32
	Model   rl.Model
	Texture rl.Texture2D
}

func setupPlayer(_ float32, app *illusion.App) {
	bi, _ := illusion.QuerySingle[*jolt.BodyInterface](app)

	playerBody := bi.CreateBody(
		jolt.CreateBox(jolt.Vec3{X: 1, Y: 1, Z: 1}),
		jolt.Vec3{X: 0, Y: 0, Z: 0},
		jolt.MotionTypeDynamic,
		false,
	)
	bi.ActivateBody(playerBody)

	player := illusion.NewComponent[Player]()
	position := illusion.NewComponent[Position]()
	velocity := illusion.NewComponent[Velocity]()

	model := rl.LoadModel("./examples/assets/bee.obj")
	// defer rl.UnloadModel(model)

	texture := rl.LoadTexture("./examples/assets/palette.png")
	// defer rl.UnloadTexture(texture)

	player.Add(player.Entity, Player{ID: playerBody, Speed: 2, Model: model, Texture: texture})

	position.Add(player.Entity, Position{X: 0, Y: 0.5, Z: 0})
	velocity.Add(player.Entity, Velocity{X: 0, Y: 0, Z: 0})

	app.AddComponent(player)
	app.AddComponent(position)
	app.AddComponent(velocity)
}

func updatePlayerSystem(delta float32, app *illusion.App) {
	bi, _ := illusion.QuerySingle[*jolt.BodyInterface](app)
	ps, _ := illusion.QuerySingle[*jolt.PhysicsSystem](app)

	ps.Update(delta)

	rl.DrawText("Hello World", 10, 40, 20, rl.LightGray)

	player, entity := illusion.QuerySingle[Player](app)
	position := bi.GetPosition(player.ID).Mul(2.0)

	var size float32 = 1.0

	rl.SetMaterialTexture(player.Model.Materials, rl.MapDiffuse, player.Texture)

	fmt.Printf("Entity %s moved to [%.2f, %.2f, %.2f]\n", *entity, position.X, position.Y, position.Z)

	rl.DrawModel(player.Model, rl.Vector3(position), size, rl.White)

	rl.DrawGrid(1000, 2.0) // Grid helps with perspective
}
