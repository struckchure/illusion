//go:build js

package render

// Browsers use WebGL 2, whose shaders are GLSL ES 3.00; fragment shaders must
// declare a default float precision: high, which the shadow map's depths
// need.
const (
	vertexHeader   = "#version 300 es"
	fragmentHeader = "#version 300 es\nprecision highp float;"
)
