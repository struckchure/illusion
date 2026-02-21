package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/samber/do"
	"github.com/samber/lo"
)

const (
	WALK_SPEED = 2.0
	JUMP_SPEED = 6.0

	ACC_RATE  = 0.2
	DECC_RATE = 0.1

	SMOOTH_FACTOR = 0.1
)

var playerSize float32 = 1.0

type Player struct {
	model   *rl.Model
	texture *rl.Texture2D

	Position *rl.Vector3
	Velocity *rl.Vector3
}

func (p *Player) Startup() {
	p.model = lo.ToPtr(rl.LoadModel("./examples/assets/bee.obj"))
	p.texture = lo.ToPtr(rl.LoadTexture("./examples/assets/palette.png"))
}

func (p *Player) Shutdown() {
	rl.UnloadModel(*p.model)
	rl.UnloadTexture(*p.texture)
}

func (p *Player) input() {
	p.Velocity.X = 0
	p.Velocity.Y = 0
	p.Velocity.Z = 0

	// Reset Y position if on ground
	p.Position.Y = rl.Lerp(p.Position.Y, 1, DECC_RATE)

	// Check Keys
	// Note: In 3D, 'W' usually means moving "Forward" into the screen (-Z)
	if rl.IsKeyDown(rl.KeyW) || rl.IsKeyDown(rl.KeyUp) {
		p.Velocity.Z = -WALK_SPEED
	}
	if rl.IsKeyDown(rl.KeyS) || rl.IsKeyDown(rl.KeyDown) {
		p.Velocity.Z = WALK_SPEED
	}
	if rl.IsKeyDown(rl.KeyA) || rl.IsKeyDown(rl.KeyLeft) {
		p.Velocity.X = -WALK_SPEED
	}
	if rl.IsKeyDown(rl.KeyD) || rl.IsKeyDown(rl.KeyRight) {
		p.Velocity.X = WALK_SPEED
	}

	if rl.IsKeyPressed(rl.KeySpace) {
		p.Velocity.Y = JUMP_SPEED
	}
}

func (p *Player) Update(dt float32) {
	p.input()

	p.Position.X += p.Velocity.X * ACC_RATE
	p.Position.Y += p.Velocity.Y * ACC_RATE
	p.Position.Z += p.Velocity.Z * ACC_RATE

	rl.SetMaterialTexture(p.model.Materials, rl.MapDiffuse, *p.texture)
	rl.DrawModel(*p.model, *p.Position, playerSize, rl.White)
}

func NewPlayer(i *do.Injector) (*Player, error) {
	return &Player{
		model:   nil,
		texture: nil,

		Position: lo.ToPtr(rl.Vector3{X: 0, Y: 0, Z: 0}),
		Velocity: lo.ToPtr(rl.Vector3{X: 0, Y: 0, Z: 0}),
	}, nil
}
