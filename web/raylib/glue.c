// Wrappers that let Go (running as its own wasm module) call raylib. JS can
// only pass numbers, so structs arrive as pointers into raylib's heap (Go
// packs them there, see bridge.go), colors arrive packed into one uint32, and
// struct results are written through an out pointer. Functions that only take
// and return numbers are exported directly instead (see ../lib.sh).

#include <stdlib.h>
#include <string.h>

#include <emscripten/emscripten.h>

#include "raylib.h"
#include "rlgl.h"

#define API EMSCRIPTEN_KEEPALIVE

static Color color(unsigned int c) {
	return (Color){c & 0xff, (c >> 8) & 0xff, (c >> 16) & 0xff, c >> 24};
}

// Core

API void w_ClearBackground(unsigned int c) { ClearBackground(color(c)); }
API void w_BeginMode3D(const Camera3D *c) { BeginMode3D(*c); }
API void w_BeginMode2D(const Camera2D *c) { BeginMode2D(*c); }
API void w_GetMousePosition(Vector2 *out) { *out = GetMousePosition(); }
API void w_GetMouseDelta(Vector2 *out) { *out = GetMouseDelta(); }

API void w_GetScreenToWorldRay(const Vector2 *p, const Camera3D *c, Ray *out) {
	*out = GetScreenToWorldRay(*p, *c);
}
API void w_GetWorldToScreen(const Vector3 *p, const Camera3D *c, Vector2 *out) {
	*out = GetWorldToScreen(*p, *c);
}
API void w_GetScreenToWorld2D(const Vector2 *p, const Camera2D *c, Vector2 *out) {
	*out = GetScreenToWorld2D(*p, *c);
}
API void w_GetWorldToScreen2D(const Vector2 *p, const Camera2D *c, Vector2 *out) {
	*out = GetWorldToScreen2D(*p, *c);
}

// Shapes

API void w_DrawLine(int x1, int y1, int x2, int y2, unsigned int c) { DrawLine(x1, y1, x2, y2, color(c)); }
API void w_DrawLineV(const Vector2 *a, const Vector2 *b, unsigned int c) { DrawLineV(*a, *b, color(c)); }
API void w_DrawLine3D(const Vector3 *a, const Vector3 *b, unsigned int c) { DrawLine3D(*a, *b, color(c)); }
API void w_DrawCircle(int x, int y, float r, unsigned int c) { DrawCircle(x, y, r, color(c)); }
API void w_DrawRectangle(int x, int y, int w, int h, unsigned int c) { DrawRectangle(x, y, w, h, color(c)); }
API void w_DrawRectangleLines(int x, int y, int w, int h, unsigned int c) { DrawRectangleLines(x, y, w, h, color(c)); }
API void w_DrawRectangleRec(const Rectangle *r, unsigned int c) { DrawRectangleRec(*r, color(c)); }
API void w_DrawRectanglePro(const Rectangle *r, const Vector2 *origin, float rotation, unsigned int c) {
	DrawRectanglePro(*r, *origin, rotation, color(c));
}
API void w_DrawCube(const Vector3 *p, float w, float h, float l, unsigned int c) { DrawCube(*p, w, h, l, color(c)); }
API void w_DrawCubeWires(const Vector3 *p, float w, float h, float l, unsigned int c) { DrawCubeWires(*p, w, h, l, color(c)); }
API void w_DrawSphere(const Vector3 *p, float r, unsigned int c) { DrawSphere(*p, r, color(c)); }
API void w_DrawPlane(const Vector3 *p, const Vector2 *size, unsigned int c) { DrawPlane(*p, *size, color(c)); }

// Text. Fonts stay in raylib's heap; Go passes a Font* (NULL for the
// default font) and mirrors the glyph metrics (see text.go).

static Font fontOf(const Font *f) { return f ? *f : GetFontDefault(); }

API void w_DrawText(const char *text, int x, int y, int size, unsigned int c) { DrawText(text, x, y, size, color(c)); }
API void w_MeasureTextEx(const Font *f, const char *text, float size, float spacing, Vector2 *out) {
	*out = MeasureTextEx(fontOf(f), text, size, spacing);
}
API void w_DrawTextEx(const Font *f, const char *text, const Vector2 *pos, float size, float spacing, unsigned int c) {
	DrawTextEx(fontOf(f), text, *pos, size, spacing, color(c));
}
API void w_DrawTextPro(const Font *f, const char *text, const Vector2 *pos, const Vector2 *origin, float rotation, float size, float spacing, unsigned int c) {
	DrawTextPro(fontOf(f), text, *pos, *origin, rotation, size, spacing, color(c));
}
API void w_DrawTextCodepoint(const Font *f, int codepoint, const Vector2 *pos, float size, unsigned int c) {
	DrawTextCodepoint(fontOf(f), codepoint, *pos, size, color(c));
}
API int w_GetGlyphIndex(const Font *f, int codepoint) { return GetGlyphIndex(fontOf(f), codepoint); }
API void w_GetGlyphAtlasRec(const Font *f, int codepoint, Rectangle *out) { *out = GetGlyphAtlasRec(fontOf(f), codepoint); }

typedef struct {
	int baseSize, glyphCount, glyphPadding;
	Texture2D texture;
	Rectangle *recs;
	GlyphInfo *glyphs;
} WFontInfo;

API void w_FontInfo(const Font *f, WFontInfo *out) {
	Font font = fontOf(f);
	*out = (WFontInfo){font.baseSize, font.glyphCount, font.glyphPadding, font.texture, font.recs, font.glyphs};
}

static Font *keepFont(Font f) {
	if (!IsFontValid(f)) return NULL;
	Font *p = malloc(sizeof *p);
	*p = f;
	return p;
}

API Font *w_LoadFont(const char *path) { return keepFont(LoadFont(path)); }
API Font *w_LoadFontEx(const char *path, int size, int *codepoints, int count) {
	return keepFont(LoadFontEx(path, size, codepoints, count));
}
API Font *w_LoadFontFromMemory(const char *type, const unsigned char *data, int dataSize, int size, int *codepoints, int count) {
	return keepFont(LoadFontFromMemory(type, data, dataSize, size, codepoints, count));
}
API Font *w_LoadFontFromImage(void *data, int w, int h, int mipmaps, int format, unsigned int key, int firstChar) {
	return keepFont(LoadFontFromImage((Image){data, w, h, mipmaps, format}, color(key), firstChar));
}

API void w_UnloadFont(Font *f) {
	UnloadFont(*f);
	free(f);
}

// Textures. Texture2D holds only numbers, so it crosses as is.

API void w_LoadTexture(const char *path, Texture2D *out) { *out = LoadTexture(path); }
API void w_LoadTextureFromImage(void *data, int width, int height, int mipmaps, int format, Texture2D *out) {
	*out = LoadTextureFromImage((Image){data, width, height, mipmaps, format});
}
API void w_UnloadTexture(const Texture2D *t) { UnloadTexture(*t); }
API void w_SetTextureFilter(const Texture2D *t, int filter) { SetTextureFilter(*t, filter); }
API void w_GenTextureMipmaps(Texture2D *t) { GenTextureMipmaps(t); }
API void w_DrawTexturePro(const Texture2D *t, const Rectangle *src, const Rectangle *dst, const Vector2 *origin, float rotation, unsigned int c) {
	DrawTexturePro(*t, *src, *dst, *origin, rotation, color(c));
}

// Meshes stay in raylib's heap; Go holds the pointer (see models.go).

static Mesh *keep(Mesh m) {
	Mesh *p = malloc(sizeof *p);
	*p = m;
	return p;
}

API Mesh *w_GenMeshCube(float w, float h, float l) { return keep(GenMeshCube(w, h, l)); }
API Mesh *w_GenMeshPlane(float w, float l, int rx, int rz) { return keep(GenMeshPlane(w, l, rx, rz)); }
API Mesh *w_GenMeshSphere(float r, int rings, int slices) { return keep(GenMeshSphere(r, rings, slices)); }
API Mesh *w_GenMeshCylinder(float r, float h, int slices) { return keep(GenMeshCylinder(r, h, slices)); }
API Mesh *w_GenMeshTorus(float r, float size, int radSeg, int sides) { return keep(GenMeshTorus(r, size, radSeg, sides)); }
API Mesh *w_GenMeshPoly(int sides, float r) { return keep(GenMeshPoly(sides, r)); }
API Mesh *w_GenMeshHemiSphere(float r, int rings, int slices) { return keep(GenMeshHemiSphere(r, rings, slices)); }
API Mesh *w_GenMeshCone(float r, float h, int slices) { return keep(GenMeshCone(r, h, slices)); }
API Mesh *w_GenMeshKnot(float r, float size, int radSeg, int sides) { return keep(GenMeshKnot(r, size, radSeg, sides)); }
API Mesh *w_GenMeshHeightmap(void *data, int w, int h, int mipmaps, int format, const Vector3 *size) {
	return keep(GenMeshHeightmap((Image){data, w, h, mipmaps, format}, *size));
}
API Mesh *w_GenMeshCubicmap(void *data, int w, int h, int mipmaps, int format, const Vector3 *size) {
	return keep(GenMeshCubicmap((Image){data, w, h, mipmaps, format}, *size));
}

// A mesh built in Go: Go copies each array it has into a fresh allocation
// (NULL when absent), and the mesh takes ownership, so UnloadMesh frees them.
typedef struct {
	int vertexCount, triangleCount;
	float *vertices, *texcoords, *texcoords2, *normals, *tangents;
	unsigned char *colors;
	unsigned short *indices;
	int boneCount;
	unsigned char *boneIndices;
	float *boneWeights;
} WMeshArrays;

static float *copyFloats(const float *src, int n) {
	float *dst = RL_CALLOC(n, sizeof(float));
	if (src) memcpy(dst, src, n * sizeof(float));
	return dst;
}

API Mesh *w_UploadMesh(const WMeshArrays *a, int dynamic) {
	Mesh m = {
		.vertexCount = a->vertexCount, .triangleCount = a->triangleCount,
		.vertices = a->vertices, .texcoords = a->texcoords, .texcoords2 = a->texcoords2,
		.normals = a->normals, .tangents = a->tangents, .colors = a->colors, .indices = a->indices,
		.boneCount = a->boneCount, .boneIndices = a->boneIndices, .boneWeights = a->boneWeights,
	};
	// Skinned meshes need buffers for CPU skinning to write, as raylib's
	// model loaders allocate.
	if (m.boneIndices && m.boneWeights) {
		m.animVertices = copyFloats(m.vertices, 3 * m.vertexCount);
		m.animNormals = copyFloats(m.normals, 3 * m.vertexCount);
	}
	UploadMesh(&m, dynamic);
	return keep(m);
}

API void w_MeshArrays(const Mesh *m, WMeshArrays *out) {
	*out = (WMeshArrays){m->vertexCount, m->triangleCount, m->vertices, m->texcoords, m->texcoords2,
		m->normals, m->tangents, m->colors, m->indices, m->boneCount, m->boneIndices, m->boneWeights};
}

API void w_MeshVboIds(const Mesh *m, unsigned int *out, int n) { memcpy(out, m->vboId, n * sizeof *out); }

API void w_UpdateMeshBuffer(const Mesh *m, int index, const void *data, int size, int offset) {
	UpdateMeshBuffer(*m, index, data, size, offset);
}
API void w_GetMeshBoundingBox(const Mesh *m, BoundingBox *out) { *out = GetMeshBoundingBox(*m); }
API void w_GenMeshTangents(Mesh *m) { GenMeshTangents(m); }
API float *w_MeshTangents(const Mesh *m) { return m->tangents; }
API int w_ExportMesh(const Mesh *m, const char *path) { return ExportMesh(*m, path); }

// The vertex arrays Go mirrors (NULL when absent), so physics can read them.
typedef struct {
	int vertexCount, triangleCount;
	unsigned int vaoId;
	float *vertices, *texcoords, *normals;
	unsigned short *indices;
	int boneCount;
	unsigned char *boneIndices;
	float *boneWeights;
} WMeshInfo;

API void w_MeshInfo(const Mesh *m, WMeshInfo *out) {
	*out = (WMeshInfo){m->vertexCount, m->triangleCount, m->vaoId, m->vertices, m->texcoords, m->normals, m->indices,
		m->boneCount, m->boneIndices, m->boneWeights};
}

API void w_UnloadMesh(Mesh *m) {
	UnloadMesh(*m);
	free(m);
}

// Materials live in Go memory (so rl.Material.GetMap works as usual) and are
// packed into this layout for each call. MaterialMap has the same layout in Go
// and C.
typedef struct {
	unsigned int shader;
	int locs[RL_MAX_SHADER_LOCATIONS];
	MaterialMap maps[12];
	float params[4];
} WMaterial;

_Static_assert(RL_MAX_SHADER_LOCATIONS == 32, "bridge assumes 32 shader locations");

static Material material(WMaterial *w) {
	Material m = {.shader = {w->shader, w->locs}, .maps = w->maps};
	memcpy(m.params, w->params, sizeof m.params);
	return m;
}

API void w_DrawMesh(const Mesh *m, WMaterial *w, const Matrix *transform) {
	DrawMesh(*m, material(w), *transform);
}

API void w_LoadMaterialDefault(WMaterial *out) {
	Material m = LoadMaterialDefault();
	out->shader = m.shader.id;
	memcpy(out->locs, m.shader.locs, sizeof out->locs);
	memcpy(out->maps, m.maps, sizeof out->maps);
	memcpy(out->params, m.params, sizeof out->params);
	RL_FREE(m.maps); // the default shader's locs belong to rlgl
}

// Like UnloadMaterial, without freeing arrays that live on the Go side.
API void w_UnloadMaterial(const WMaterial *w) {
	if (w->shader != rlGetShaderIdDefault()) rlUnloadShaderProgram(w->shader);
	for (int i = 0; i < 12; i++) {
		unsigned int id = w->maps[i].texture.id;
		if (id != 0 && id != rlGetTextureIdDefault()) rlUnloadTexture(id);
	}
}

// Shaders: Go keeps its own copy of the locations array.

API unsigned int w_LoadShaderFromMemory(const char *vs, const char *fs, int *locs) {
	Shader s = LoadShaderFromMemory(vs, fs);
	memcpy(locs, s.locs, RL_MAX_SHADER_LOCATIONS * sizeof(int));
	if (s.locs != rlGetShaderLocsDefault()) RL_FREE(s.locs);
	return s.id;
}

API void w_UnloadShader(unsigned int id) {
	if (id != rlGetShaderIdDefault()) rlUnloadShaderProgram(id);
}

API void w_SetShaderValue(unsigned int id, int loc, const void *value, int type) {
	SetShaderValue((Shader){id, NULL}, loc, value, type);
}
API void w_SetShaderValueV(unsigned int id, int loc, const void *value, int type, int count) {
	SetShaderValueV((Shader){id, NULL}, loc, value, type, count);
}
API void w_SetShaderValueMatrix(unsigned int id, int loc, const Matrix *m) {
	SetShaderValueMatrix((Shader){id, NULL}, loc, *m);
}

// Render targets and the matrices drawn with (for shadow maps).

API void w_BeginTextureMode(const RenderTexture2D *t) { BeginTextureMode(*t); }
API void w_GetMatrixModelview(Matrix *out) { *out = rlGetMatrixModelview(); }
API void w_GetMatrixProjection(Matrix *out) { *out = rlGetMatrixProjection(); }
API int w_FramebufferComplete(unsigned int id) { return rlFramebufferComplete(id); }
API unsigned int w_LoadTextureDepth(int width, int height, int useRenderBuffer) {
	return rlLoadTextureDepth(width, height, useRenderBuffer);
}

// Models stay in raylib's heap; Go mirrors their meshes (registered like the
// generated ones), materials and mesh-material table (see models.go).

API Model *w_LoadModel(const char *path) {
	Model *m = malloc(sizeof *m);
	*m = LoadModel(path);
	return m;
}

typedef struct {
	int meshCount, materialCount;
	Matrix transform;
	int *meshMaterial;
} WModelInfo;

API void w_ModelInfo(const Model *m, WModelInfo *out) {
	*out = (WModelInfo){m->meshCount, m->materialCount, m->transform, m->meshMaterial};
}

API Mesh *w_ModelMesh(const Model *m, int i) { return &m->meshes[i]; }

API void w_ModelMaterial(const Model *m, int i, WMaterial *out) {
	Material mat = m->materials[i];
	out->shader = mat.shader.id;
	memcpy(out->locs, mat.shader.locs, sizeof out->locs);
	memcpy(out->maps, mat.maps, sizeof out->maps);
	memcpy(out->params, mat.params, sizeof out->params);
}

API int w_IsModelValid(const Model *m) { return IsModelValid(*m); }
API void w_GetModelBoundingBox(const Model *m, BoundingBox *out) { *out = GetModelBoundingBox(*m); }

API void w_UnloadModel(Model *m) {
	UnloadModel(*m);
	free(m);
}

// Sounds and music stay in raylib's heap; Go keys each by a placeholder in
// its Stream.Buffer (see audio.go).

typedef struct {
	unsigned int frameCount, sampleRate, sampleSize, channels;
} WAudioInfo;

static Sound *keepSound(Sound s) {
	if (!IsSoundValid(s)) return NULL;
	Sound *p = malloc(sizeof *p);
	*p = s;
	return p;
}

API Sound *w_LoadSound(const char *path) { return keepSound(LoadSound(path)); }
API Sound *w_LoadSoundAlias(const Sound *s) { return keepSound(LoadSoundAlias(*s)); }
API Sound *w_LoadSoundFromWave(unsigned int frames, unsigned int rate, unsigned int size, unsigned int channels, void *data) {
	return keepSound(LoadSoundFromWave((Wave){frames, rate, size, channels, data})); // copies data
}

API void w_SoundInfo(const Sound *s, WAudioInfo *out) {
	*out = (WAudioInfo){s->frameCount, s->stream.sampleRate, s->stream.sampleSize, s->stream.channels};
}

API void w_UnloadSound(Sound *s) {
	UnloadSound(*s);
	free(s);
}
API void w_UnloadSoundAlias(Sound *s) {
	UnloadSoundAlias(*s);
	free(s);
}

API void w_PlaySound(const Sound *s) { PlaySound(*s); }
API void w_StopSound(const Sound *s) { StopSound(*s); }
API void w_PauseSound(const Sound *s) { PauseSound(*s); }
API void w_ResumeSound(const Sound *s) { ResumeSound(*s); }
API int w_IsSoundPlaying(const Sound *s) { return IsSoundPlaying(*s); }
API void w_SetSoundVolume(const Sound *s, float v) { SetSoundVolume(*s, v); }
API void w_SetSoundPitch(const Sound *s, float v) { SetSoundPitch(*s, v); }
API void w_SetSoundPan(const Sound *s, float v) { SetSoundPan(*s, v); }

// Music.looping lives on the Go side, where games set it, so calls that read
// it take it as an argument.

API Music *w_LoadMusicStream(const char *path) {
	Music m = LoadMusicStream(path);
	if (!IsMusicValid(m)) return NULL;
	Music *p = malloc(sizeof *p);
	*p = m;
	return p;
}

API void w_MusicInfo(const Music *m, WAudioInfo *out) {
	*out = (WAudioInfo){m->frameCount, m->stream.sampleRate, m->stream.sampleSize, m->stream.channels};
}

API void w_UnloadMusicStream(Music *m) {
	UnloadMusicStream(*m);
	free(m);
}

API void w_PlayMusicStream(const Music *m) { PlayMusicStream(*m); }
API void w_StopMusicStream(const Music *m) { StopMusicStream(*m); }
API void w_PauseMusicStream(const Music *m) { PauseMusicStream(*m); }
API void w_ResumeMusicStream(const Music *m) { ResumeMusicStream(*m); }
API int w_IsMusicStreamPlaying(const Music *m) { return IsMusicStreamPlaying(*m); }
API void w_UpdateMusicStream(Music *m, int looping) {
	m->looping = looping;
	UpdateMusicStream(*m);
}
API void w_SeekMusicStream(const Music *m, float position) { SeekMusicStream(*m, position); }
API void w_SetMusicVolume(const Music *m, float v) { SetMusicVolume(*m, v); }
API void w_SetMusicPitch(const Music *m, float v) { SetMusicPitch(*m, v); }
API void w_SetMusicPan(const Music *m, float v) { SetMusicPan(*m, v); }
API float w_GetMusicTimeLength(const Music *m) { return GetMusicTimeLength(*m); }
API float w_GetMusicTimePlayed(const Music *m) { return GetMusicTimePlayed(*m); }

// Images live in Go memory. Drawing into one copies its pixels here, runs
// raylib's drawing code on them, and copies them back (see images.go).

API void w_ImageDrawCircle(void *data, int w, int h, int mipmaps, int format, int cx, int cy, int r, unsigned int c) {
	Image img = {data, w, h, mipmaps, format};
	ImageDrawCircle(&img, cx, cy, r, color(c));
}
API void w_ImageDrawRectangle(void *data, int w, int h, int mipmaps, int format, int x, int y, int rw, int rh, unsigned int c) {
	Image img = {data, w, h, mipmaps, format};
	ImageDrawRectangle(&img, x, y, rw, rh, color(c));
}

static unsigned int pack(Color c) { return c.r | c.g << 8 | c.b << 16 | (unsigned int)c.a << 24; }

API unsigned int w_ColorFromHSV(float h, float s, float v) { return pack(ColorFromHSV(h, s, v)); }

API void w_DrawRectangleRounded(const Rectangle *r, float roundness, int segments, unsigned int c) {
	DrawRectangleRounded(*r, roundness, segments, color(c));
}

API Model *w_LoadModelFromMesh(const Mesh *mesh) {
	Model *p = malloc(sizeof *p);
	*p = LoadModelFromMesh(*mesh);
	return p;
}

// Skeletons and animations. The model's skeleton, current pose and bone
// matrices stay here; Go mirrors them and re-reads the pose after each
// animation update (see animation.go).

typedef struct {
	int boneCount;
	BoneInfo *bones;
	Transform *bindPose, *currentPose;
	Matrix *boneMatrices;
} WSkeletonInfo;

API void w_ModelSkeleton(const Model *m, WSkeletonInfo *out) {
	*out = (WSkeletonInfo){m->skeleton.boneCount, m->skeleton.bones, m->skeleton.bindPose, m->currentPose, m->boneMatrices};
}

API ModelAnimation *w_LoadModelAnimations(const char *path, int *count) { return LoadModelAnimations(path, count); }
API void w_UnloadModelAnimations(ModelAnimation *anims, int count) { UnloadModelAnimations(anims, count); }
API int w_IsModelAnimationValid(const Model *m, const ModelAnimation *a) { return IsModelAnimationValid(*m, *a); }

// raylib 6.0's CPU skinning uploads the posed normals to
// vboId[SHADER_LOC_VERTEX_NORMAL], the colour slot (3), instead of the normal
// slot (2), so they never reach the GPU and WebGL reports "bufferSubData: no
// buffer" every frame (see render/skinning.go). routeNormals points each
// skinned mesh's colour slot at its normal buffer for the update, keeping the
// real one in saved; restoreColors puts it back.
static void routeNormals(const Model *m, unsigned int *saved) {
	for (int i = 0; i < m->meshCount; i++) {
		Mesh *mesh = &m->meshes[i];
		saved[i] = mesh->vboId[3];
		if (mesh->animNormals && mesh->boneWeights && mesh->vboId[2]) mesh->vboId[3] = mesh->vboId[2];
	}
}
static void restoreColors(const Model *m, const unsigned int *saved) {
	for (int i = 0; i < m->meshCount; i++) m->meshes[i].vboId[3] = saved[i];
}

API void w_UpdateModelAnimation(const Model *m, const ModelAnimation *a, float frame) {
	unsigned int *saved = malloc(m->meshCount * sizeof(unsigned int));
	routeNormals(m, saved);
	UpdateModelAnimation(*m, *a, frame);
	restoreColors(m, saved);
	free(saved);
}
API void w_UpdateModelAnimationEx(const Model *m, const ModelAnimation *a, float frameA, const ModelAnimation *b, float frameB, float blend) {
	unsigned int *saved = malloc(m->meshCount * sizeof(unsigned int));
	routeNormals(m, saved);
	UpdateModelAnimationEx(*m, *a, frameA, *b, frameB, blend);
	restoreColors(m, saved);
	free(saved);
}

// For shaders that skin on the GPU: uploads the model's bone matrices, as
// DrawModelEx does before each mesh.
API void w_SetBoneMatrices(const Model *m, unsigned int shader, int loc) {
	rlEnableShader(shader);
	rlSetUniformMatrices(loc, m->boneMatrices, m->skeleton.boneCount);
}
