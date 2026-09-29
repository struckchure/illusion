//go:build js

package rl

import (
	"image/color"
	"unsafe"
)

// Images live in Go memory: Data points at a Go byte slice, and UnloadImage
// leaves it to the garbage collector. Functions that change pixels copy them
// into raylib's heap, run raylib's code there and copy them back, so results
// match desktop pixel for pixel.

// GenImageColor returns an RGBA image filled with col.
func GenImageColor(width, height int, col color.RGBA) *Image {
	data := make([]byte, 4*width*height)
	for i := 0; i < len(data); i += 4 {
		data[i], data[i+1], data[i+2], data[i+3] = col.R, col.G, col.B, col.A
	}
	return NewImage(data, int32(width), int32(height), 1, UncompressedR8g8b8a8)
}

// UnloadImage does nothing on the web: the pixels are Go memory.
func UnloadImage(image *Image) {}

// imageBytes returns the image's pixel data, mipmaps included.
func imageBytes(image *Image) []byte {
	size := 0
	w, h := image.Width, image.Height
	for range max(1, image.Mipmaps) {
		size += call("GetPixelDataSize", w, h, int32(image.Format)).Int()
		w, h = max(1, w/2), max(1, h/2)
	}
	return unsafe.Slice((*byte)(image.Data), size)
}

// editImage runs the C function fn on a copy of dst's pixels in raylib's
// heap, followed by args, and copies the result back.
func editImage(dst *Image, fn string, args ...any) {
	data := imageBytes(dst)
	p := malloc(data)
	defer free(p)
	call(fn, append([]any{p, dst.Width, dst.Height, dst.Mipmaps, int32(dst.Format)}, args...)...)
	readHeap(p, data)
}

func ImageDrawCircle(dst *Image, centerX, centerY, radius int32, col color.RGBA) {
	editImage(dst, "w_ImageDrawCircle", centerX, centerY, radius, rgba(col))
}

func ImageDrawRectangle(dst *Image, posX, posY, width, height int32, col color.RGBA) {
	editImage(dst, "w_ImageDrawRectangle", posX, posY, width, height, rgba(col))
}
