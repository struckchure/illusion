package illusion

type SystemType string

const (
	Update  SystemType = "update"
	Startup SystemType = "startup"
)

type SystemFunc func(delta float32, app *App)

type System struct {
	Type SystemType
	Func SystemFunc
}
