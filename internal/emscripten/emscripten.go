//go:build js

// Package emscripten calls C functions in a wasm module built with
// emscripten (-sMODULARIZE, exporting _malloc, _free and HEAPU8) from Go's own
// wasm module.
//
// The two modules don't share memory. Arguments passed by pointer are copied
// into a scratch block in the C module's heap (or a temporary allocation when
// they don't fit), and results are read back from there. A call's pointers
// stay valid until arguments for the next call start being packed:
//
//	out := m.Alloc(12)
//	m.Call("get_position", body, out)
//	pos := emscripten.Read[[3]float32](m, out)
//
// Types copied with Arg, Slice and Read must have the same layout in Go and
// in wasm32 C: fixed-size numbers and arrays of them, no pointers, int or
// uintptr (which are 64-bit in Go's wasm).
package emscripten

import (
	"syscall/js"
	"unsafe"
)

const scratchSize = 64 << 10

// Module is a loaded emscripten module.
type Module struct {
	name    string
	v       js.Value
	exports map[string]js.Value

	scratch uint32   // address of the scratch block in the module's heap
	heap    js.Value // HEAPU8, refreshed when the module's memory grows
	view    js.Value // the scratch block within heap

	buf    [scratchSize]byte // Go-side copy of the scratch block
	used   int               // bytes of buf packed for the next call
	temps  []uint32          // allocations to free before the next call
	called bool              // the packed arguments belong to a finished call
}

// Load returns the module the page stored at globalThis[global] (the
// resolved result of its factory function).
func Load(global string) *Module {
	v := js.Global().Get(global)
	if v.IsUndefined() {
		panic("emscripten: globalThis." + global + " is not set; load and await the module before starting Go")
	}
	m := &Module{name: global, v: v, exports: map[string]js.Value{}}
	m.scratch = uint32(v.Call("_malloc", scratchSize).Int())
	return m
}

// heapView returns HEAPU8, recreating the views if the heap grew (growing
// replaces the ArrayBuffer and detaches views of the old one).
func (m *Module) heapView() js.Value {
	if m.heap.IsUndefined() || m.heap.Get("byteLength").Int() == 0 {
		m.heap = m.v.Get("HEAPU8")
		m.view = m.heap.Call("subarray", m.scratch, m.scratch+scratchSize)
	}
	return m.heap
}

// begin starts packing arguments for a new call, releasing the last call's.
func (m *Module) begin() {
	if !m.called {
		return
	}
	m.called = false
	m.used = 0
	for _, p := range m.temps {
		m.v.Call("_free", p)
	}
	m.temps = m.temps[:0]
}

// Alloc reserves n bytes in the module's heap for the next call to write a
// result to, and returns their address.
func (m *Module) Alloc(n int) uint32 {
	m.begin()
	off := (m.used + 7) &^ 7
	if off+n <= scratchSize {
		m.used = off + n
		return m.scratch + uint32(off)
	}
	p := uint32(m.v.Call("_malloc", n).Int())
	m.temps = append(m.temps, p)
	return p
}

// Bytes copies b into the module's heap for the next call and returns its
// address.
func (m *Module) Bytes(b []byte) uint32 {
	p := m.Alloc(len(b))
	if off := int(p - m.scratch); p >= m.scratch && off+len(b) <= scratchSize {
		copy(m.buf[off:], b)
	} else {
		js.CopyBytesToJS(m.heapView().Call("subarray", p, p+uint32(len(b))), b)
	}
	return p
}

// String copies s into the module's heap as a NUL-terminated C string.
func (m *Module) String(s string) uint32 {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return m.Bytes(b)
}

// Arg copies v into the module's heap and returns its address.
func Arg[T any](m *Module, v T) uint32 {
	return m.Bytes(unsafe.Slice((*byte)(unsafe.Pointer(&v)), unsafe.Sizeof(v)))
}

// Slice copies s's elements into the module's heap and returns the address
// of the first.
func Slice[T any](m *Module, s []T) uint32 {
	if len(s) == 0 {
		return m.Bytes(nil)
	}
	return m.Bytes(unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), uintptr(len(s))*unsafe.Sizeof(s[0])))
}

// Call calls the exported C function name (without emscripten's leading
// underscore).
func (m *Module) Call(name string, args ...any) js.Value {
	f, ok := m.exports[name]
	if !ok {
		f = m.v.Get("_" + name)
		if f.Type() != js.TypeFunction {
			panic("emscripten: " + m.name + " does not export " + name)
		}
		m.exports[name] = f
	}
	m.begin()
	if m.used > 0 {
		m.heapView()
		js.CopyBytesToJS(m.view, m.buf[:m.used])
	}
	m.called = true
	return f.Invoke(args...)
}

// ReadInto copies len(dst) bytes at address p into dst.
func (m *Module) ReadInto(p uint32, dst []byte) {
	js.CopyBytesToGo(dst, m.heapView().Call("subarray", p, p+uint32(len(dst))))
}

// Read returns the T at address p.
func Read[T any](m *Module, p uint32) T {
	var v T
	m.ReadInto(p, unsafe.Slice((*byte)(unsafe.Pointer(&v)), unsafe.Sizeof(v)))
	return v
}

// ReadSlice fills dst with the elements at address p.
func ReadSlice[T any](m *Module, p uint32, dst []T) {
	if len(dst) == 0 {
		return
	}
	m.ReadInto(p, unsafe.Slice((*byte)(unsafe.Pointer(&dst[0])), uintptr(len(dst))*unsafe.Sizeof(dst[0])))
}

// Uint32 converts a C unsigned int or pointer result. JS sees wasm's i32
// results as signed.
func Uint32(v js.Value) uint32 { return uint32(v.Int()) }

// Bool converts a C int or bool result.
func Bool(v js.Value) bool { return v.Int() != 0 }

// Int converts a Go bool to a C int.
func Int(b bool) int {
	if b {
		return 1
	}
	return 0
}
