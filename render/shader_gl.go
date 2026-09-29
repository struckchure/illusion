//go:build !js

package render

// Desktop raylib uses OpenGL 3.3.
const (
	vertexHeader   = "#version 330"
	fragmentHeader = "#version 330"
)
