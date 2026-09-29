# Shared by build.sh, test.sh and go.sh. Expects $web (this directory) and
# $mod (the Go module whose packages are built: illusion itself, or a game
# that depends on it) to be set.
#
# Illusion may be a read-only copy in the Go module cache, so nothing is
# written here: compiled C objects go to the user cache directory
# (ILLUSION_WEB_CACHE overrides it), and the Go workspace to a temporary
# directory.

illusion=$(cd "$web/.." && pwd)
cache=${ILLUSION_WEB_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/illusion/web}

# The browser workspace: $mod as is, with raylib-go replaced by web/raylib.
# web/raylib has no go.mod of its own (a nested module would be left out of
# illusion's module download), so a copy gets one here.
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir "$work/raylib"
cp "$web"/raylib/*.go "$work/raylib/"
printf 'module github.com/gen2brain/raylib-go/raylib\n\ngo 1.25\n' >"$work/raylib/go.mod"
{
	echo "go $(go env GOVERSION | sed 's/^go//')"
	echo
	echo "use \"$mod\"" # quoted: paths may contain spaces
	echo
	echo "replace github.com/gen2brain/raylib-go/raylib => \"$work/raylib\""
} >"$work/browser.work"
export GOWORK=$work/browser.work

# uses_jolt <package>...: whether any of the packages depends on Jolt.
uses_jolt() {
	(cd "$mod" && GOOS=js GOARCH=wasm go list -deps "$@") | grep -qx github.com/struckchure/illusion/internal/jolt
}

# checksum <file>...: a short content hash, to key cached objects built from
# files that change between illusion versions.
checksum() {
	cat "$@" | cksum | cut -d' ' -f1
}

# compile <object> <source> <flags...>: compiles the source unless the object
# exists. Object paths are keyed by version or checksum, so they never go
# stale. (sh has no local variables, hence the prefixed names.)
compile() {
	c_obj=$1 c_src=$2
	shift 2
	if [ ! -f "$c_obj" ]; then
		echo "emcc $(basename "$c_src")"
		emcc "$@" -c "$c_src" -o "$c_obj"
	fi
}

# Link flags shared by every module. env is web for pages, node for tests.
link_flags() {
	echo "-sMODULARIZE=1 -sENVIRONMENT=$1 -sALLOW_MEMORY_GROWTH=1 --no-entry"
}

# build_raylib <out-dir> <env> [emcc flags...]: raylib, from the same sources
# raylib-go vendors, plus web/raylib/glue.c. Extra flags go to the link (for
# --preload-file). The module exports its FS for web/fs.js.
build_raylib() {
	b_out=$1 b_env=$2
	shift 2
	version=$(cat "$web/raylib/UPSTREAM_VERSION")
	(cd "$work" && GOWORK=off GOFLAGS= go mod download github.com/gen2brain/raylib-go/raylib@"$version")
	src=$(go env GOMODCACHE)/github.com/gen2brain/raylib-go/raylib@$version
	# $flags is word-split, so paths (which may contain spaces) go in quoted.
	flags="-Os -DPLATFORM_WEB -DGRAPHICS_API_OPENGL_ES3 -sUSE_GLFW=3 -Wno-unused-value"
	objs=$cache/raylib-$version
	mkdir -p "$objs"
	for m in rcore rshapes rtextures rtext rmodels raudio; do
		compile "$objs/$m.o" "$src/$m.c" $flags -I"$src"
	done
	glue=$cache/raylib-glue-$(checksum "$web/raylib/glue.c").o
	compile "$glue" "$web/raylib/glue.c" $flags -I"$src"

	# Functions Go calls directly (numbers in, numbers out). The w_* wrappers
	# in glue.c are exported by EMSCRIPTEN_KEEPALIVE.
	exports=_malloc,_free
	for f in SetConfigFlags SetTraceLogLevel InitWindow CloseWindow IsWindowReady \
		IsWindowFocused SetWindowTitle GetScreenWidth GetScreenHeight SetExitKey \
		GetFPS GetFrameTime GetTime BeginDrawing EndDrawing EndMode3D EndMode2D \
		IsKeyDown IsKeyPressed IsKeyReleased GetKeyPressed GetCharPressed \
		IsMouseButtonDown IsMouseButtonPressed IsMouseButtonReleased GetMouseWheelMove \
		DrawGrid DrawFPS MeasureText GetPixelDataSize rlGetTextureIdDefault \
		rlGetLocationUniform InitAudioDevice CloseAudioDevice IsAudioDeviceReady \
		SetMasterVolume; do
		exports=$exports,_$f
	done
	echo "link raylib.wasm"
	emcc $flags -I"$src" "$objs"/*.o "$glue" -o "$b_out/raylib.js" $(link_flags "$b_env") \
		-sEXPORT_NAME=createRaylib -sMIN_WEBGL_VERSION=2 -sMAX_WEBGL_VERSION=2 \
		-sEXPORTED_FUNCTIONS="$exports" -sFORCE_FILESYSTEM=1 -sEXPORTED_RUNTIME_METHODS=HEAPU8,FS "$@"
}

# build_jolt <out-dir> <env>: Jolt and internal/jolt/glue.cpp,
# single-threaded, with wasm SIMD.
build_jolt() {
	b_out=$1 b_env=$2
	jolt=$illusion/internal/jolt
	flags="-std=c++17 -O2 -DNDEBUG -msimd128 -msse4.2 -Wno-unused-parameter"
	objs=$cache/jolt-$(cut -d' ' -f3 "$jolt/third_party/VERSION")
	mkdir -p "$objs"
	for f in "$jolt"/unity_*.cpp; do
		compile "$objs/$(basename "$f" .cpp).o" "$f" $flags -I"$jolt" -I"$jolt/third_party/JoltPhysics"
	done
	glue=$cache/jolt-glue-$(checksum "$jolt/glue.cpp" "$jolt/glue.h").o
	compile "$glue" "$jolt/glue.cpp" $flags -I"$jolt" -I"$jolt/third_party/JoltPhysics"
	exports=_malloc,_free$(grep -o 'ILL_[A-Za-z_]*(' "$jolt/glue.h" | tr -d '(' | sort -u | sed 's/^/,_/' | tr -d '\n')
	echo "link jolt.wasm"
	em++ $flags "$objs"/*.o "$glue" -o "$b_out/jolt.js" $(link_flags "$b_env") \
		-sEXPORT_NAME=createJolt -sEXPORTED_FUNCTIONS="$exports" -sEXPORTED_RUNTIME_METHODS=HEAPU8 \
		-sSTACK_SIZE=1MB # emscripten's default 64KB overflows during a physics step
}
