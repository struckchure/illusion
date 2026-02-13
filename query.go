package illusion

func Query[T any](app *App) *Component[T] {
	for _, component := range app.Components {
		component, ok := component.(*Component[T])
		if !ok {
			continue
		}

		return component
	}

	return nil
}

func QuerySingle[T any](app *App) (T, *Entity) {
	parent := Query[T](app)

	for entity := range parent.Data {
		if v, ok := parent.Get(entity); ok {
			return *v, &entity
		}
	}

	var zero T

	return zero, nil
}

func QuerySingleWith[T, U any](app *App) (*Entity, *T, *U) {
	parent := Query[T](app)
	target := Query[U](app)

	for entity, parent := range parent.Data {
		if child, ok := target.Get(entity); ok {
			return &entity, parent, child
		}
	}

	return nil, nil, nil
}

type Relation[T, U any] struct {
	Parent *T
	Child  *U
}

func QueryWith[T, U any](app *App) []*Relation[T, U] {
	var relations []*Relation[T, U]

	parent := Query[T](app)
	target := Query[U](app)

	for entity, parent := range parent.Data {
		if child, ok := target.Get(entity); ok {
			relations = append(relations, &Relation[T, U]{Parent: parent, Child: child})
		}
	}

	return relations
}
