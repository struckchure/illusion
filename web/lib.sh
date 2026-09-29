# Shared by build.sh and test.sh: builds the C libraries as emscripten
# modules. Expects $root (the repository) and $web (this directory) to be set.
# Objects are cached in web/.cache, so only the first build is slow.

export GOWORK=$web/browser.work
cache=$web/.cache

# uses_jolt <package>...: whether any of the packages depends on Jolt.
uses_jolt() {
	(cd "$root" && GOOS=js GOARCH=wasm go list -deps "$@") | grep -qx github.com/struckchure/illusion/internal/jolt
}

# compile <object> <source> <flags...>: compiles the source unless the object
# is newer. (sh has no local variables, hence the prefixed names.)
compile() {
	c_obj=$1 c_src=$2
	shift 2
	if [ ! "$c_obj" -nt "$c_src" ]; then
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
	(cd "$root" && GOWORK=off go mod download github.com/gen2brain/raylib-go/raylib)
	src=$(go env GOMODCACHE)/github.com/gen2brain/raylib-go/raylib@$version
	# $flags is word-split, so paths (which may contain spaces) go in quoted.
	flags="-Os -DPLATFORM_WEB -DGRAPHICS_API_OPENGL_ES3 -sUSE_GLFW=3 -Wno-unused-value"
	objs=$cache/raylib-$version
	mkdir -p "$objs"
	for m in rcore rshapes rtextures rtext rmodels raudio; do
		compile "$objs/$m.o" "$src/$m.c" $flags -I"$src"
	done
	compile "$objs/glue.o" "$web/raylib/glue.c" $flags -I"$src"

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
	emcc $flags -I"$src" "$objs"/*.o -o "$b_out/raylib.js" $(link_flags "$b_env") \
		-sEXPORT_NAME=createRaylib -sMIN_WEBGL_VERSION=2 -sMAX_WEBGL_VERSION=2 \
		-sEXPORTED_FUNCTIONS="$exports" -sFORCE_FILESYSTEM=1 -sEXPORTED_RUNTIME_METHODS=HEAPU8,FS "$@"
}

# build_jolt <out-dir> <env>: Jolt and internal/jolt/glue.cpp,
# single-threaded, with wasm SIMD.
build_jolt() {
	b_out=$1 b_env=$2
	jolt=$root/internal/jolt
	flags="-std=c++17 -O2 -DNDEBUG -msimd128 -msse4.2 -Wno-unused-parameter"
	objs=$cache/jolt-$(cut -d' ' -f3 "$jolt/third_party/VERSION")
	mkdir -p "$objs"
	# compile only compares an object with its source; glue.cpp also depends
	# on glue.h (Jolt's own sources are keyed by version in $objs).
	if [ "$jolt/glue.h" -nt "$objs/glue.o" ]; then
		rm -f "$objs/glue.o"
	fi
	for f in "$jolt"/unity_*.cpp "$jolt/glue.cpp"; do
		compile "$objs/$(basename "$f" .cpp).o" "$f" $flags -I"$jolt" -I"$jolt/third_party/JoltPhysics"
	done
	exports=_malloc,_free$(grep -o 'ILL_[A-Za-z_]*(' "$jolt/glue.h" | tr -d '(' | sort -u | sed 's/^/,_/' | tr -d '\n')
	echo "link jolt.wasm"
	em++ $flags "$objs"/*.o -o "$b_out/jolt.js" $(link_flags "$b_env") \
		-sEXPORT_NAME=createJolt -sEXPORTED_FUNCTIONS="$exports" -sEXPORTED_RUNTIME_METHODS=HEAPU8 \
		-sSTACK_SIZE=1MB # emscripten's default 64KB overflows during a physics step
}
