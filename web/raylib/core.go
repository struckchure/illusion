//go:build js

package rl

import (
	"image/color"
	"syscall/js"
)

func truthy(v js.Value) bool { return v.Int() != 0 }

// Window

func SetConfigFlags(flags uint32)             { call("SetConfigFlags", flags) }
func SetTraceLogLevel(logLevel TraceLogLevel) { call("SetTraceLogLevel", int(logLevel)) }

func InitWindow(width int32, height int32, title string) {
	call("InitWindow", width, height, argString(title))
}

func CloseWindow()                { call("CloseWindow") }
func IsWindowReady() bool         { return truthy(call("IsWindowReady")) }
func IsWindowFocused() bool       { return truthy(call("IsWindowFocused")) }
func SetWindowTitle(title string) { call("SetWindowTitle", argString(title)) }
func GetScreenWidth() int         { return call("GetScreenWidth").Int() }
func GetScreenHeight() int        { return call("GetScreenHeight").Int() }
func SetExitKey(key int32)        { call("SetExitKey", key) }

// WindowShouldClose is always false in a browser: the page owns the canvas,
// and raylib's web version of this function needs Asyncify.
func WindowShouldClose() bool { return false }

// SetTargetFPS does nothing in a browser, where requestAnimationFrame paces
// frames to the display.
func SetTargetFPS(fps int32) {}

// Timing

func GetFPS() int32         { return int32(call("GetFPS").Int()) }
func GetFrameTime() float32 { return float32(call("GetFrameTime").Float()) }
func GetTime() float64      { return call("GetTime").Float() }

// Drawing

func BeginDrawing()                  { call("BeginDrawing") }
func EndDrawing()                    { call("EndDrawing") }
func ClearBackground(col color.RGBA) { call("w_ClearBackground", rgba(col)) }
func BeginMode3D(camera Camera3D)    { call("w_BeginMode3D", arg(camera)) }
func EndMode3D()                     { call("EndMode3D") }
func BeginMode2D(camera Camera2D)    { call("w_BeginMode2D", arg(camera)) }
func EndMode2D()                     { call("EndMode2D") }

func GetScreenToWorldRay(position Vector2, camera Camera) Ray {
	call("w_GetScreenToWorldRay", arg(position), arg(camera), out())
	return result[Ray]()
}

func GetWorldToScreen(position Vector3, camera Camera) Vector2 {
	call("w_GetWorldToScreen", arg(position), arg(camera), out())
	return result[Vector2]()
}

func GetScreenToWorld2D(position Vector2, camera Camera2D) Vector2 {
	call("w_GetScreenToWorld2D", arg(position), arg(camera), out())
	return result[Vector2]()
}

func GetWorldToScreen2D(position Vector2, camera Camera2D) Vector2 {
	call("w_GetWorldToScreen2D", arg(position), arg(camera), out())
	return result[Vector2]()
}

// Input

func IsKeyDown(key int32) bool     { return truthy(call("IsKeyDown", key)) }
func IsKeyPressed(key int32) bool  { return truthy(call("IsKeyPressed", key)) }
func IsKeyReleased(key int32) bool { return truthy(call("IsKeyReleased", key)) }
func GetKeyPressed() int32         { return int32(call("GetKeyPressed").Int()) }
func GetCharPressed() int32        { return int32(call("GetCharPressed").Int()) }

func IsMouseButtonDown(button MouseButton) bool {
	return truthy(call("IsMouseButtonDown", int32(button)))
}

func IsMouseButtonPressed(button MouseButton) bool {
	return truthy(call("IsMouseButtonPressed", int32(button)))
}

func IsMouseButtonReleased(button MouseButton) bool {
	return truthy(call("IsMouseButtonReleased", int32(button)))
}

func GetMousePosition() Vector2 {
	call("w_GetMousePosition", out())
	return result[Vector2]()
}

func GetMouseDelta() Vector2 {
	call("w_GetMouseDelta", out())
	return result[Vector2]()
}

func GetMouseWheelMove() float32 { return float32(call("GetMouseWheelMove").Float()) }

// Colors

func Fade(col color.RGBA, alpha float32) color.RGBA { return ColorAlpha(col, alpha) }

func ColorFromHSV(hue, saturation, value float32) color.RGBA {
	c := uint32(call("w_ColorFromHSV", hue, saturation, value).Int())
	return color.RGBA{R: uint8(c), G: uint8(c >> 8), B: uint8(c >> 16), A: uint8(c >> 24)}
}

// Collisions

func CheckCollisionPointRec(point Vector2, rec Rectangle) bool {
	return point.X >= rec.X && point.X < rec.X+rec.Width && point.Y >= rec.Y && point.Y < rec.Y+rec.Height
}

func ColorAlpha(col color.RGBA, alpha float32) color.RGBA {
	alpha = max(0, min(1, alpha))
	col.A = uint8(255 * alpha)
	return col
}
