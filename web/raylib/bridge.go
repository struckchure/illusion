//go:build js

package rl

import (
	"image/color"
	"syscall/js"
	"unsafe"
)

// raylib runs as a separate emscripten module (see glue.c and ../lib.sh), with
// its own linear memory that Go can't point into. Calls go through
// syscall/js, and anything passed by pointer is copied through a scratch
// block malloc'd once in raylib's heap: results are written at its start, and
// arguments are packed after that (or into a temporary allocation when they
// don't fit, freed after the call).
const (
	scratchSize = 64 << 10
	outSize     = 1 << 10
)

var (
	module  js.Value                // the emscripten module, globalThis.raylib
	exports = map[string]js.Value{} // cached module exports, by C name
	scratch uint32                  // address of the scratch block in raylib's heap
	heap    js.Value                // raylib's HEAPU8, refreshed when memory grows
	outView js.Value                // the result area of the scratch block
	argView js.Value                // the argument area of the scratch block

	buf   [scratchSize]byte // Go-side copy of the scratch block
	used  int               // bytes of arguments packed after outSize
	temps []uint32          // arguments too big for the scratch block
)

// load finds the module on first use rather than at init, so code that only
// uses rl's types and math (tests, say) runs without raylib loaded.
func load() {
	if scratch != 0 {
		return
	}
	module = js.Global().Get("raylib")
	if module.IsUndefined() {
		panic("rl: globalThis.raylib is not set; load raylib.js and await it before starting Go")
	}
	scratch = uint32(module.Call("_malloc", scratchSize).Int())
}

// views returns raylib's heap, recreating the typed-array views if the heap
// grew (growing replaces the ArrayBuffer and detaches the old views). Checking
// costs a JS call, so it's only done before copying data, not on every call.
func views() js.Value {
	load()
	if heap.IsUndefined() || heap.Get("byteLength").Int() == 0 {
		heap = module.Get("HEAPU8")
		outView = heap.Call("subarray", scratch, scratch+outSize)
		argView = heap.Call("subarray", scratch+outSize, scratch+scratchSize)
	}
	return heap
}

func export(name string) js.Value {
	f, ok := exports[name]
	if !ok {
		f = module.Get("_" + name)
		if f.Type() != js.TypeFunction {
			panic("rl: raylib.wasm does not export " + name + "; add it to web/lib.sh")
		}
		exports[name] = f
	}
	return f
}

// call flushes the packed arguments and calls a C function by name.
func call(name string, args ...any) js.Value {
	load()
	if used > 0 {
		views()
		js.CopyBytesToJS(argView, buf[outSize:outSize+used])
		used = 0
	}
	r := export(name).Invoke(args...)
	for _, p := range temps {
		free(p)
	}
	temps = temps[:0]
	return r
}

// out is the address C functions write their result to.
func out() uint32 {
	load()
	return scratch
}

// result reads a C function's result from the scratch block. T must have the
// same layout in Go and C: fixed-size numbers only, no pointers.
func result[T any]() T {
	var v T
	views()
	js.CopyBytesToGo(unsafe.Slice((*byte)(unsafe.Pointer(&v)), unsafe.Sizeof(v)), outView)
	return v
}

// arg packs v into the scratch block and returns its address in raylib's heap.
// T must have the same layout in Go and C: fixed-size numbers only, no
// pointers.
func arg[T any](v T) uint32 {
	return argBytes(unsafe.Slice((*byte)(unsafe.Pointer(&v)), unsafe.Sizeof(v)))
}

func argSlice[T any](s []T) uint32 {
	if len(s) == 0 {
		return argBytes(nil)
	}
	return argBytes(unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), uintptr(len(s))*unsafe.Sizeof(s[0])))
}

func argBytes(b []byte) uint32 {
	load()
	off := (outSize + used + 7) &^ 7
	if off+len(b) > scratchSize {
		p := malloc(b)
		temps = append(temps, p)
		return p
	}
	copy(buf[off:], b)
	used = off + len(b) - outSize
	return scratch + uint32(off)
}

// argString packs s as a NUL-terminated C string.
func argString(s string) uint32 {
	load()
	off := (outSize + used + 7) &^ 7
	if off+len(s)+1 > scratchSize {
		b := make([]byte, len(s)+1)
		copy(b, s)
		return argBytes(b)
	}
	copy(buf[off:], s)
	buf[off+len(s)] = 0
	used = off + len(s) + 1 - outSize
	return scratch + uint32(off)
}

// readHeap copies len(dst) bytes at address p in raylib's heap into dst.
func readHeap(p uint32, dst []byte) {
	js.CopyBytesToGo(dst, views().Call("subarray", p, p+uint32(len(dst))))
}

// writeHeap copies b to address p in raylib's heap.
func writeHeap(p uint32, b []byte) {
	js.CopyBytesToJS(views().Call("subarray", p, p+uint32(len(b))), b)
}

// readArg reads back an argument the C function changed in place.
func readArg[T any](p uint32) T {
	var v T
	readHeap(p, unsafe.Slice((*byte)(unsafe.Pointer(&v)), unsafe.Sizeof(v)))
	return v
}

// mirror returns a Go copy of n Ts at address p, or nil when p is NULL.
func mirror[T any](p uint32, n int) *T {
	if p == 0 || n <= 0 {
		return nil
	}
	s := make([]T, n)
	readHeap(p, unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), uintptr(n)*unsafe.Sizeof(s[0])))
	return &s[0]
}

// malloc copies b into a fresh allocation in raylib's heap. Free it with free.
func malloc(b []byte) uint32 {
	load()
	p := uint32(module.Call("_malloc", len(b)).Int())
	js.CopyBytesToJS(views().Call("subarray", p, p+uint32(len(b))), b)
	return p
}

func free(p uint32) { module.Call("_free", p) }

// rgba packs a color the way glue.c unpacks it.
func rgba(c color.RGBA) uint32 {
	return uint32(c.R) | uint32(c.G)<<8 | uint32(c.B)<<16 | uint32(c.A)<<24
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
