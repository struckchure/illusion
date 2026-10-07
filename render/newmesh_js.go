//go:build js

package render

import "unsafe"

// meshArray is s, for the browser's UploadMesh to copy into raylib's heap:
// nil for an empty s.
func meshArray[T any](s []T) unsafe.Pointer {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Pointer(&s[0])
}
