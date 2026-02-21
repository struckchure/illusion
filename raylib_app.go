package illusion

import (
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type RaylibApp struct {
	App *App
}

func (a *RaylibApp) Run() {
	fps := 60
	width := 800
	height := 450
	title := "Illusion"

	rl.InitWindow(int32(width), int32(height), title)

	defer rl.CloseWindow()
	rl.SetTargetFPS(int32(fps))

	for !rl.WindowShouldClose() {
		rl.BeginDrawing()
		rl.ClearBackground(rl.DarkGray)

		dt := time.Duration(float32(time.Nanosecond)*rl.GetFrameTime()) / 100
		a.App.Loop(dt)

		rl.EndMode3D()
		rl.EndDrawing()
	}
}

func NewRaylibApp() *RaylibApp {
	return &RaylibApp{App: NewApp()}
}
