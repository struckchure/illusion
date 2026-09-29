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

// Text. Only raylib's default font is supported so far.

func DrawFPS(posX int32, posY int32) { call("DrawFPS", posX, posY) }

func DrawText(text string, posX int32, posY int32, fontSize int32, col color.RGBA) {
	call("w_DrawText", argString(text), posX, posY, fontSize, rgba(col))
}

func MeasureText(text string, fontSize int32) int32 {
	return int32(call("MeasureText", argString(text), fontSize).Int())
}

// GetFontDefault returns raylib's default font. On the web its glyph arrays
// stay in raylib's memory, so Recs and Chars are nil.
func GetFontDefault() Font {
	call("w_GetFontDefault", out())
	f := result[struct {
		BaseSize, GlyphCount, GlyphPadding int32
		Texture                            Texture2D
	}]()
	return Font{BaseSize: f.BaseSize, CharsCount: f.GlyphCount, CharsPadding: f.GlyphPadding, Texture: f.Texture}
}

// MeasureTextEx measures with the default font; font is ignored on the web.
func MeasureTextEx(font Font, text string, fontSize float32, spacing float32) Vector2 {
	call("w_MeasureTextEx", argString(text), fontSize, spacing, out())
	return result[Vector2]()
}

// DrawTextPro draws with the default font; font is ignored on the web.
func DrawTextPro(font Font, text string, position, origin Vector2, rotation, fontSize, spacing float32, tint color.RGBA) {
	call("w_DrawTextPro", argString(text), arg(position), arg(origin), rotation, fontSize, spacing, rgba(tint))
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

func IsTextureValid(texture Texture2D) bool {
	return texture.ID > 0 && texture.Width > 0 && texture.Height > 0 && texture.Format > 0 && texture.Mipmaps > 0
}

func GetTextureIdDefault() uint32 { return uint32(call("rlGetTextureIdDefault").Int()) }

func DrawTexturePro(texture Texture2D, source, dest Rectangle, origin Vector2, rotation float32, tint color.RGBA) {
	call("w_DrawTexturePro", arg(texture), arg(source), arg(dest), arg(origin), rotation, rgba(tint))
}
