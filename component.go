package illusion

// Component is a generic container for ANY type of data.
// We use [T any] so you can store Position, Health, RenderData, etc.
type Component[T any] struct {
	Entity Entity
	Data   map[Entity]*T
}

func NewComponent[T any]() *Component[T] {
	return &Component[T]{
		Entity: NewEntity(),
		Data:   make(map[Entity]*T),
	}
}

// Add links a data component to an entity
func (m *Component[T]) Add(e Entity, component T) {
	m.Data[e] = &component
}

// Get retrieves the data (if it exists)
func (m *Component[T]) Get(e Entity) (*T, bool) {
	val, ok := m.Data[e]
	return val, ok
}

// Remove deletes the component
func (m *Component[T]) Remove(e Entity) {
	delete(m.Data, e)
}
