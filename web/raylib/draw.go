//go:build js

package rl

import "image/color"

// Shapes

func DrawGrid(slices int32, spacing float32) { call("DrawGrid", slices, spacing) }

func DrawLine(startPosX, startPosY, endPosX, endPosY int32, col color.RGBA) {
	call("w_DrawLine", startPosX, startPosY, endPosX, endPosY, rgba(col))
}

func DrawLineV(startPos, endPos Vector2, col color.RGBA) {
	call("w_DrawLineV", arg(startPos), arg(endPos), rgba(col))
}

func DrawLine3D(startPos Vector3, endPos Vector3, col color.RGBA) {
	call("w_DrawLine3D", arg(startPos), arg(endPos), rgba(col))
}

func DrawCircle(centerX, centerY int32, radius float32, col color.RGBA) {
	call("w_DrawCircle", centerX, centerY, radius, rgba(col))
}

func DrawRectangle(posX, posY, width, height int32, col color.RGBA) {
	call("w_DrawRectangle", posX, posY, width, height, rgba(col))
}

func DrawRectangleLines(posX, posY, width, height int32, col color.RGBA) {
	call("w_DrawRectangleLines", posX, posY, width, height, rgba(col))
}

func DrawRectangleRec(rec Rectangle, col color.RGBA) {
	call("w_DrawRectangleRec", arg(rec), rgba(col))
}

func DrawRectangleRounded(rec Rectangle, roundness float32, segments int32, col color.RGBA) {
	call("w_DrawRectangleRounded", arg(rec), roundness, segments, rgba(col))
}

func DrawRectanglePro(rec Rectangle, origin Vector2, rotation float32, col color.RGBA) {
	call("w_DrawRectanglePro", arg(rec), arg(origin), rotation, rgba(col))
}

func DrawCube(position Vector3, width float32, height float32, length float32, col color.RGBA) {
	call("w_DrawCube", arg(position), width, height, length, rgba(col))
}

func DrawCubeWires(position Vector3, width float32, height float32, length float32, col color.RGBA) {
	call("w_DrawCubeWires", arg(position), width, height, length, rgba(col))
}

func DrawSphere(centerPos Vector3, radius float32, col color.RGBA) {
	call("w_DrawSphere", arg(centerPos), radius, rgba(col))
}

func DrawPlane(centerPos Vector3, size Vector2, col color.RGBA) {
	call("w_DrawPlane", arg(centerPos), arg(size), rgba(col))
}

// Textures

// LoadTexture loads from raylib's virtual filesystem: files must be bundled
// with the page (emcc --preload-file).
func LoadTexture(fileName string) Texture2D {
	call("w_LoadTexture", argString(fileName), out())
	return result[Texture2D]()
}

func LoadTextureFromImage(image *Image) Texture2D {
	data := malloc(imageBytes(image))
	defer free(data)
	call("w_LoadTextureFromImage", data, image.Width, image.Height, image.Mipmaps, int32(image.Format), out())
	return result[Texture2D]()
}

func UnloadTexture(texture Texture2D) { call("w_UnloadTexture", arg(texture)) }

func SetTextureFilter(texture Texture2D, filter TextureFilterMode) {
	call("w_SetTextureFilter", arg(texture), int32(filter))
}

func GenTextureMipmaps(texture *Texture2D) {
	p := arg(*texture)
	call("w_GenTextureMipmaps", p)
	*texture = readArg[Texture2D](p)
}

func IsTextureValid(texture Texture2D) bool {
	return texture.ID > 0 && texture.Width > 0 && texture.Height > 0 && texture.Format > 0 && texture.Mipmaps > 0
}

// Render targets, and the rlgl calls that make them (for shadow maps).

func BeginTextureMode(target RenderTexture2D) { call("w_BeginTextureMode", arg(target)) }
func EndTextureMode()                         { call("EndTextureMode") }
func LoadFramebuffer() uint32                 { return uint32(call("rlLoadFramebuffer").Int()) }
func UnloadFramebuffer(id uint32)             { call("rlUnloadFramebuffer", id) }
func FramebufferComplete(id uint32) bool      { return truthy(call("w_FramebufferComplete", id)) }

func LoadTextureDepth(width, height int32, useRenderBuffer bool) uint32 {
	b := 0
	if useRenderBuffer {
		b = 1
	}
	return uint32(call("w_LoadTextureDepth", width, height, b).Int())
}

func FramebufferAttach(id, texId uint32, attachType, texType, mipLevel int32) {
	call("rlFramebufferAttach", id, texId, attachType, texType, mipLevel)
}

// SetClipPlanes sets the near and far planes BeginMode3D projects with.
func SetClipPlanes(nearPlane, farPlane float64) { call("rlSetClipPlanes", nearPlane, farPlane) }
func GetCullDistanceNear() float64              { return call("rlGetCullDistanceNear").Float() }
func GetCullDistanceFar() float64               { return call("rlGetCullDistanceFar").Float() }

func ActiveTextureSlot(slot int32) { call("rlActiveTextureSlot", slot) }
func EnableTexture(id uint32)      { call("rlEnableTexture", id) }
func DisableTexture()              { call("rlDisableTexture") }

func GetMatrixModelview() Matrix {
	call("w_GetMatrixModelview", out())
	return result[Matrix]()
}

func GetMatrixProjection() Matrix {
	call("w_GetMatrixProjection", out())
	return result[Matrix]()
}

// SetCullFace sets which faces are culled: 0 for front faces, 1 for back.
func SetCullFace(mode int32) { call("rlSetCullFace", mode) }

func GetTextureIdDefault() uint32 { return uint32(call("rlGetTextureIdDefault").Int()) }

func DrawTexturePro(texture Texture2D, source, dest Rectangle, origin Vector2, rotation float32, tint color.RGBA) {
	call("w_DrawTexturePro", arg(texture), arg(source), arg(dest), arg(origin), rotation, rgba(tint))
}

// CheckCollisionRecs reports whether two rectangles overlap, as raylib's.
func CheckCollisionRecs(rec1, rec2 Rectangle) bool {
	return rec1.X < rec2.X+rec2.Width && rec1.X+rec1.Width > rec2.X &&
		rec1.Y < rec2.Y+rec2.Height && rec1.Y+rec1.Height > rec2.Y
}
