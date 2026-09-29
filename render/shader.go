package render

// The default material shader: textured, vertex-colored, with one
// directional light plus ambient. Uniform and attribute names follow raylib's
// defaults, so raylib binds mvp, matModel, matNormal, colDiffuse and texture0.

const litVertexShader = `#version 330
in vec3 vertexPosition;
in vec2 vertexTexCoord;
in vec3 vertexNormal;
in vec4 vertexColor;

uniform mat4 mvp;
uniform mat4 matNormal;

out vec2 fragTexCoord;
out vec4 fragColor;
out vec3 fragNormal;

void main() {
    fragTexCoord = vertexTexCoord;
    fragColor = vertexColor;
    fragNormal = normalize(vec3(matNormal * vec4(vertexNormal, 0.0)));
    gl_Position = mvp * vec4(vertexPosition, 1.0);
}
`

const litFragmentShader = `#version 330
in vec2 fragTexCoord;
in vec4 fragColor;
in vec3 fragNormal;

uniform sampler2D texture0;
uniform vec4 colDiffuse;
uniform vec3 lightDir;
uniform vec3 lightColor;
uniform vec3 ambient;
uniform float unlit;

out vec4 finalColor;

void main() {
    vec4 base = texture(texture0, fragTexCoord) * colDiffuse * fragColor;
    if (unlit > 0.5) {
        finalColor = base;
        return;
    }
    float diffuse = max(dot(normalize(fragNormal), -lightDir), 0.0);
    finalColor = vec4(base.rgb * (ambient + lightColor * diffuse), base.a);
}
`
