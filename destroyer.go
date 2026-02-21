package illusion

import (
	"sync"
)

type DestroyerSystem struct {
	fns []func()
}

func (d *DestroyerSystem) Startup() {}

func (d *DestroyerSystem) Shutdown() {
	for _, fn := range d.fns {
		fn()
	}
}

func (d *DestroyerSystem) Update(dt float32) {}

var (
	Destroyer     DestroyerSystem
	onceDestroyer sync.Once
)

func Destroy(fn func()) {
	onceDestroyer.Do(func() {
		Destroyer = DestroyerSystem{}
	})

	Destroyer.fns = append(Destroyer.fns, fn)
}
