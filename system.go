package illusion

type System interface {
	Startup()
	Shutdown()
	Update(delta float32)
}
