//go:build js

package rl

import (
	"image/color"
	"unsafe"
)

// Fonts stay in raylib's heap. The Go Font mirrors the glyph rectangles and
// metrics (Recs, Chars) and keys the C pointer here by its Recs array; a Font
// that isn't registered (GetFontDefault's, or the zero Font) draws with the
// default font. Glyph images aren't mirrored: Chars[i].Image has the size but
// no Data.
var (
	fonts       = map[*Rectangle]uint32{}
	defaultFont *Font
)

// wFontInfo matches WFontInfo in glue.c.
type wFontInfo struct {
	BaseSize, GlyphCount, GlyphPadding int32
	Texture                            Texture2D
	Recs, Glyphs                       uint32
}

// cGlyphInfo is GlyphInfo in wasm32 C, where Image.Data is 4 bytes.
type cGlyphInfo struct {
	Value, OffsetX, OffsetY, AdvanceX int32
	Data                              uint32
	Width, Height, Mipmaps            int32
	Format                            PixelFormat
}

// mirrorFont builds the Go side of the font at p (0 for the default font).
func mirrorFont(p uint32) Font {
	call("w_FontInfo", p, out())
	info := result[wFontInfo]()
	n := int(info.GlyphCount)
	f := Font{
		BaseSize:     info.BaseSize,
		CharsCount:   info.GlyphCount,
		CharsPadding: info.GlyphPadding,
		Texture:      info.Texture,
		Recs:         mirror[Rectangle](info.Recs, n),
	}
	if c := mirror[cGlyphInfo](info.Glyphs, n); c != nil {
		glyphs := make([]GlyphInfo, n)
		for i, g := range unsafe.Slice(c, n) {
			glyphs[i] = GlyphInfo{
				Value: g.Value, OffsetX: g.OffsetX, OffsetY: g.OffsetY, AdvanceX: g.AdvanceX,
				Image: Image{Width: g.Width, Height: g.Height, Mipmaps: g.Mipmaps, Format: g.Format},
			}
		}
		f.Chars = &glyphs[0]
	}
	return f
}

func keepFont(p uint32) Font {
	if p == 0 {
		return Font{}
	}
	f := mirrorFont(p)
	fonts[f.Recs] = p
	return f
}

// font returns the C pointer to pass for f: 0 means the default font.
func font(f Font) uint32 {
	if f.Recs == nil {
		return 0
	}
	return fonts[f.Recs]
}

// GetFontDefault returns raylib's default font. It's available once the
// window is open.
func GetFontDefault() Font {
	if defaultFont == nil {
		f := mirrorFont(0)
		if f.CharsCount == 0 {
			return f // window not open yet
		}
		defaultFont = &f
	}
	return *defaultFont
}

// LoadFont loads from raylib's virtual filesystem: files must be bundled with
// the page (web/build.sh -a).
func LoadFont(fileName string) Font {
	return keepFont(uint32(call("w_LoadFont", argString(fileName)).Int()))
}

// codepoints copies runes into raylib's heap for a load call; free the
// result (0 when runes is empty).
func codepoints(runes []rune) uint32 {
	if len(runes) == 0 {
		return 0
	}
	return malloc(unsafe.Slice((*byte)(unsafe.Pointer(&runes[0])), 4*len(runes)))
}

func LoadFontEx(fileName string, fontSize int32, fontChars []rune, runesNumber ...int32) Font {
	count := int32(len(fontChars))
	if fontChars == nil && len(runesNumber) > 0 {
		count = runesNumber[0]
	}
	cp := codepoints(fontChars)
	if cp != 0 {
		defer free(cp)
	}
	return keepFont(uint32(call("w_LoadFontEx", argString(fileName), fontSize, cp, count).Int()))
}

// LoadFontFromMemory loads a font file's contents, e.g. one embedded with
// go:embed; fileType is its extension, like ".ttf".
func LoadFontFromMemory(fileType string, fileData []byte, fontSize int32, codepointList []rune) Font {
	data := malloc(fileData)
	defer free(data)
	cp := codepoints(codepointList)
	if cp != 0 {
		defer free(cp)
	}
	return keepFont(uint32(call("w_LoadFontFromMemory", argString(fileType), data, len(fileData), fontSize, cp, len(codepointList)).Int()))
}

func LoadFontFromImage(image Image, key color.RGBA, firstChar rune) Font {
	data := malloc(imageBytes(&image))
	defer free(data)
	return keepFont(uint32(call("w_LoadFontFromImage", data, image.Width, image.Height, image.Mipmaps, int32(image.Format), rgba(key), firstChar).Int()))
}

func IsFontValid(f Font) bool {
	return f.BaseSize > 0 && f.CharsCount > 0 && f.Recs != nil && f.Chars != nil
}

func UnloadFont(f Font) {
	if p := font(f); p != 0 {
		delete(fonts, f.Recs)
		call("w_UnloadFont", p)
	}
}

func DrawFPS(posX int32, posY int32) { call("DrawFPS", posX, posY) }

func DrawText(text string, posX int32, posY int32, fontSize int32, col color.RGBA) {
	call("w_DrawText", argString(text), posX, posY, fontSize, rgba(col))
}

func MeasureText(text string, fontSize int32) int32 {
	return int32(call("MeasureText", argString(text), fontSize).Int())
}

func MeasureTextEx(f Font, text string, fontSize float32, spacing float32) Vector2 {
	call("w_MeasureTextEx", font(f), argString(text), fontSize, spacing, out())
	return result[Vector2]()
}

func DrawTextEx(f Font, text string, position Vector2, fontSize float32, spacing float32, tint color.RGBA) {
	call("w_DrawTextEx", font(f), argString(text), arg(position), fontSize, spacing, rgba(tint))
}

func DrawTextPro(f Font, text string, position, origin Vector2, rotation, fontSize, spacing float32, tint color.RGBA) {
	call("w_DrawTextPro", font(f), argString(text), arg(position), arg(origin), rotation, fontSize, spacing, rgba(tint))
}

func DrawTextCodepoint(f Font, codepoint rune, position Vector2, fontSize float32, tint color.RGBA) {
	call("w_DrawTextCodepoint", font(f), codepoint, arg(position), fontSize, rgba(tint))
}

func GetGlyphIndex(f Font, codepoint rune) int32 {
	return int32(call("w_GetGlyphIndex", font(f), codepoint).Int())
}

// GetGlyphInfo returns the glyph's metrics from the Go mirror.
func GetGlyphInfo(f Font, codepoint rune) GlyphInfo {
	if f.Chars == nil {
		f = GetFontDefault()
	}
	return unsafe.Slice(f.Chars, f.CharsCount)[GetGlyphIndex(f, codepoint)]
}

func GetGlyphAtlasRec(f Font, codepoint rune) Rectangle {
	call("w_GetGlyphAtlasRec", font(f), codepoint, out())
	return result[Rectangle]()
}
