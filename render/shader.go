package render

// The default material shader: textured, vertex-colored, lit by one
// directional light (with its shadows), the point lights and ambient.
// Uniform and attribute names follow raylib's defaults, so raylib binds mvp,
// matModel, matNormal, colDiffuse and texture0. The GLSL version header
// depends on the platform: see shader_gl.go and shader_js.go.
//
// A game's own [Shader] is given the same: the attributes vertexPosition,
// vertexTexCoord, vertexNormal and vertexColor, raylib's uniforms, and
// lightDir (the way the light shines), lightColor, ambient, unlit (above 0.5
// for a StandardMaterial that is Unlit) and viewPos (the camera's position),
// for those it declares. Put [LightingGLSL] in it for the shadows, the point
// lights and emissive surfaces.

// LightingGLSL declares, for a fragment shader, the uniforms the renderer
// sends for shadows, point lights and emissive surfaces, and functions that
// use them:
//
//   - sunShadow(pos, n, toLight): how much of the directional light reaches
//     pos, a point on a surface facing n, from 0 (none) to 1 (all);
//     toLight is -lightDir.
//   - pointCount and pointLight(i, pos, n): how many point lights there are,
//     and how much light number i gives pos, facing n, from 0 to 1 (its
//     colour is pointColor[i]); pointLights(pos, n) is all of their light.
//   - emissive: the light the surface drawn gives off (black for none).
const LightingGLSL = `
uniform mat4 lightSpace;
uniform highp sampler2D shadowMap;
uniform vec4 shadowParams; // on, a texel in the map, a texel in metres, the depth bias
uniform float pointCount;
uniform vec4 pointPos[8]; // where, and the reach in w
uniform vec3 pointColor[8];
uniform vec3 emissive;

// shadowTap is how lit uv is at depth z in the shadow map, compared at
// the four texels round it and blended between them, so shadows' edges are
// smooth rather than stepped.
float shadowTap(vec2 uv, float z) {
    float texel = shadowParams.y;
    vec2 st = uv / texel - 0.5;
    vec2 f = fract(st);
    vec2 t = (floor(st) + 0.5) * texel;
    float a = step(z, texture(shadowMap, t).r);
    float b = step(z, texture(shadowMap, t + vec2(texel, 0.0)).r);
    float c = step(z, texture(shadowMap, t + vec2(0.0, texel)).r);
    float d = step(z, texture(shadowMap, t + vec2(texel)).r);
    return mix(mix(a, b, f.x), mix(c, d, f.x), f.y);
}

float sunShadow(vec3 pos, vec3 n, vec3 toLight) {
    if (shadowParams.x < 0.5) {
        return 1.0;
    }
    // Look up a little off the surface, more the more it's turned from the
    // light, so it doesn't shadow itself.
    float facing = clamp(dot(n, toLight), 0.0, 1.0);
    vec3 p = pos + n * shadowParams.z * (1.0 + 2.0 * (1.0 - facing));
    vec4 ls = lightSpace * vec4(p, 1.0);
    vec3 c = ls.xyz / ls.w * 0.5 + 0.5;
    vec2 off = abs(c.xy * 2.0 - 1.0);
    float edge = max(off.x, off.y);
    if (edge >= 1.0 || c.z >= 1.0) {
        return 1.0;
    }
    // Four blended taps a texel apart: a tent three texels wide.
    float z = c.z - shadowParams.w;
    float h = 0.5 * shadowParams.y;
    float lit = shadowTap(c.xy + vec2(-h, -h), z) + shadowTap(c.xy + vec2(h, -h), z)
        + shadowTap(c.xy + vec2(-h, h), z) + shadowTap(c.xy + vec2(h, h), z);
    // Fade out towards the map's edges, where it stops.
    return mix(lit / 4.0, 1.0, smoothstep(0.8, 1.0, edge));
}

float pointLight(int i, vec3 pos, vec3 n) {
    vec3 to = pointPos[i].xyz - pos;
    float d = length(to);
    float fall = max(1.0 - d / pointPos[i].w, 0.0);
    return fall * fall * max(dot(n, to / max(d, 0.0001)), 0.0);
}

vec3 pointLights(vec3 pos, vec3 n) {
    vec3 sum = vec3(0.0);
    for (int i = 0; i < 8; i++) {
        if (float(i) >= pointCount) {
            break;
        }
        sum += pointColor[i] * pointLight(i, pos, n);
    }
    return sum;
}
`

const litVertexShader = `
in vec3 vertexPosition;
in vec2 vertexTexCoord;
in vec3 vertexNormal;
in vec4 vertexColor;

uniform mat4 mvp;
uniform mat4 matModel;
uniform mat4 matNormal;

out vec3 fragPosition;
out vec2 fragTexCoord;
out vec4 fragColor;
out vec3 fragNormal;

void main() {
    fragPosition = vec3(matModel * vec4(vertexPosition, 1.0));
    fragTexCoord = vertexTexCoord;
    fragColor = vertexColor;
    fragNormal = normalize(vec3(matNormal * vec4(vertexNormal, 0.0)));
    gl_Position = mvp * vec4(vertexPosition, 1.0);
}
`

const litFragmentShader = LightingGLSL + `
in vec3 fragPosition;
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
    // Cut out what's less than half opaque: hair cards, eyebrows and the
    // like are alpha-tested, as raylib draws without sorting.
    if (base.a < 0.5) {
        discard;
    }
    if (unlit > 0.5) {
        finalColor = base;
        return;
    }
    vec3 n = normalize(fragNormal);
    float diffuse = max(dot(n, -lightDir), 0.0) * sunShadow(fragPosition, n, -lightDir);
    vec3 light = max(ambient + lightColor * diffuse + pointLights(fragPosition, n), emissive);
    finalColor = vec4(base.rgb * light, base.a);
}
`
