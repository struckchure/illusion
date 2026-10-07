//go:build !js

package render

/*
#include <stdlib.h>
#include <string.h>
*/
import "C"

import "unsafe"

// meshArray copies s into memory raylib frees (with free, as RL_FREE is):
// nil for an empty s.
func meshArray[T any](s []T) unsafe.Pointer {
	if len(s) == 0 {
		return nil
	}
	n := C.size_t(len(s)) * C.size_t(unsafe.Sizeof(s[0]))
	p := C.malloc(n)
	C.memcpy(p, unsafe.Pointer(&s[0]), n)
	return p
}
