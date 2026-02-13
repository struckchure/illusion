package illusion

import "github.com/google/uuid"

// Entity is just a unique ID
type Entity string

func NewEntity() Entity {
	return Entity(uuid.New().String())
}
