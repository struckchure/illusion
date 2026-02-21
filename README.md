# Illusion

Illusion is a lightweight, modular Go framework designed for 3D game development and simulations. It brings together the simplicity of **Raylib**, the performance of **Ark/ECS**, and the power of **Jolt Physics** into a cohesive development experience.

## Features

- **Modular System Architecture**: Organize your logic into `Systems` with managed lifecycles (`Startup`, `Update`, `Shutdown`).
- **High-Performance ECS**: Built-in integration with `ark` for efficient entity and component management.
- **3D Physics**: Native integration with the **Jolt Physics** engine (via `jolt-go`) for robust 3D simulations.
- **Flexible DI**: Designed to work seamlessly with dependency injection containers like `samber/do`.
- **Raylib Integration**: Easy-to-use 3D rendering and window management.

## Installation

```bash
go get github.com/struckchure/illusion
```

Note: Since this project uses Jolt Physics and Raylib, ensure you have the necessary CGO dependencies installed on your system (e.g., a C++ compiler for Jolt and development headers for Raylib).

## Quick Start

The core of Illusion is the `System` interface. You define your game logic by implementing these three methods:

```go
type System interface {
	Startup()
	Shutdown()
	Update(delta float32)
}
```

### Basic Example (Using Ark ECS)

Here is a simplified version of how to set up an Illusion application:

```go
package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
)

func main() {
	// 1. Initialize ECS World
	world := ecs.NewWorld()

	// 2. Initialize Raylib
	rl.InitWindow(800, 450, "Illusion Engine")
	defer rl.CloseWindow()
	rl.SetTargetFPS(60)

	// 3. Setup Systems
	systems := []illusion.System{
		illusion.NewPhysicsSystem(world),
		illusion.NewSpawnSystem(world),
		&illusion.Destroyer,
		// ... your custom systems
	}

	// 4. Startup
	for _, s := range systems {
		s.Startup()
	}
	defer func() {
		for _, s := range systems {
			s.Shutdown()
		}
	}()

	// 5. Game Loop
	for !rl.WindowShouldClose() {
		dt := rl.GetFrameTime()

		rl.BeginDrawing()
		rl.ClearBackground(rl.Black)

		// Update all systems
		for _, s := range systems {
			s.Update(dt)
		}

		rl.EndDrawing()
	}
}
```

## Key Components

### Physics System
Illusion provides a wrapper around Jolt Physics. It automatically manages the physics world and provides access to the `BodyInterface` through ECS resources.

### Spawn System
The `SpawnSystem` simplifies entity creation within the ECS. It observes entity creation and automatically calls the `Startup` method on any `System` components attached to new entities.

### Destroyer System
A utility system to handle cleanup tasks. You can register cleanup functions from anywhere in your code using `illusion.Destroy(fn)`.

## Examples

Check out the `examples/` directory for more detailed implementations:

- **`examples/vanilla`**: Basic Raylib movement without ECS.
- **`examples/di`**: Using `samber/do` for clean dependency injection.
- **`examples/ark`**: A full 3D example featuring a player, camera, and terrain using the Ark ECS and Illusion systems.

## License

This project is licensed under the MIT License - see the LICENSE file for details.