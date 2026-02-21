package illusion

import (
	"time"

	"github.com/unitoftime/ecs"
)

type App struct {
	World     *ecs.World
	Scheduler *ecs.Scheduler
}

func NewApp() *App {
	world := ecs.NewWorld()
	scheduler := ecs.NewScheduler(world)

	return &App{World: world, Scheduler: scheduler}
}

func (a *App) Loop(dt time.Duration) {
	a.Scheduler.Step(dt)
	a.Scheduler.ClearSystems(ecs.StagePreUpdate)
}
