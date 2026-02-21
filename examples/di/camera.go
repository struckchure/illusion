package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/samber/do"
)

var (
	cameraDistance float32 = 10.0
	cameraHeight   float32 = 5.0
)

type Camera struct {
	i *do.Injector
	*rl.Camera3D
}

func (c *Camera) Startup() {
	c.Camera3D = &rl.Camera3D{
		Position:   rl.NewVector3(0.0, cameraHeight, cameraDistance),
		Target:     rl.NewVector3(0.0, 0.0, 0.0),
		Up:         rl.NewVector3(0.0, 1.0, 0.0),
		Fovy:       45.0,
		Projection: rl.CameraPerspective,
	}
}

func (c *Camera) Shutdown() {
}

func (c *Camera) Update(dt float32) {
	player := do.MustInvoke[*Player](c.i)

	c.Target = rl.Vector3Lerp(c.Target, *player.Position, SMOOTH_FACTOR)
	idealPos := rl.Vector3Add(*player.Position, rl.NewVector3(0.0, cameraHeight, cameraDistance))
	c.Position = rl.Vector3Lerp(c.Position, idealPos, SMOOTH_FACTOR)
}

func NewCamera(i *do.Injector) (*Camera, error) {
	return &Camera{i: i}, nil
}
