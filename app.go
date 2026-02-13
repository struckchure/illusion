package illusion

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type App struct {
	Components []any
	systems    []System
}

func NewApp() *App {
	return &App{}
}

func (a *App) Run() {
	fps := 60
	width := 800
	height := 450
	title := "Illusion Engine"

	rl.InitWindow(int32(width), int32(height), title)
	defer rl.CloseWindow()

	rl.SetTargetFPS(int32(fps))

	fmt.Println("--- Illusion Engine Started ---")

	updates := []System{}

	for _, system := range a.systems {
		switch system.Type {
		case Startup:
			system.Func(0, a)
		case Update:
			updates = append(updates, system)
		}
	}

	for !rl.WindowShouldClose() {
		rl.BeginDrawing()
		rl.ClearBackground(rl.Black)

		for _, system := range updates {
			system.Func(rl.GetFrameTime(), a)
		}

		rl.DrawFPS(5, 5)

		rl.EndMode3D()
		rl.EndDrawing()
	}
}

func (a *App) AddComponent(component any) *App {
	a.Components = append(a.Components, component)

	return a
}

func (a *App) AddSystem(type_ SystemType, func_ SystemFunc) *App {
	a.systems = append(a.systems, System{Type: type_, Func: func_})

	return a
}
