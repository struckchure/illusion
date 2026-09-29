package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTriangulateOBJ(t *testing.T) {
	src := "mtllib kit.mtl\nv 0 0 0\nf 1 2 3 4 5\nf 1/1/1 2/2/2 3/3/3 4/4/4 5/5/5 6/6/6 7/7/7\n"
	out, changed := triangulateOBJ([]byte(src), "/assets")
	if !changed {
		t.Fatal("a 7-vertex face should be rewritten")
	}
	want := "mtllib /assets/kit.mtl\nv 0 0 0\nf 1 2 3 4 5\n" +
		"f 1/1/1 2/2/2 3/3/3\nf 1/1/1 3/3/3 4/4/4\nf 1/1/1 4/4/4 5/5/5\nf 1/1/1 5/5/5 6/6/6\nf 1/1/1 6/6/6 7/7/7\n"
	if string(out) != want {
		t.Fatalf("got\n%s\nwant\n%s", out, want)
	}

	small := "v 0 0 0\nf 1 2 3 4\n"
	if out, changed := triangulateOBJ([]byte(small), "/assets"); changed || string(out) != small {
		t.Fatal("files within tinyobj's limits should be left alone")
	}
}

// Every model in the example kit must be loadable without tripping tinyobj.
func TestKitHasNoOversizedFacesAfterSanitizing(t *testing.T) {
	files, _ := filepath.Glob("../examples/assets/*.obj")
	if len(files) == 0 {
		t.Skip("no example assets")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out, _ := triangulateOBJ(src, filepath.Dir(f))
		for _, line := range strings.Split(string(out), "\n") {
			if fields := strings.Fields(line); len(fields) > 1+maxOBJFaceVertices && fields[0] == "f" {
				t.Fatalf("%s still has a %d-vertex face", f, len(fields)-1)
			}
		}
	}
}

// The rewritten copy must sit next to the original: raylib loads a material's
// textures relative to the OBJ's directory.
func TestSanitizedOBJIsWrittenNextToTheOriginal(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "hexagon.obj")
	src := "mtllib kit.mtl\nv 0 0 0\nv 1 0 0\nv 1 1 0\nv 0 1 0\nv -1 1 0\nv -1 0 0\nf 1 2 3 4 5 6\n"
	if err := os.WriteFile(orig, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	safe, cleanup, err := sanitizeOBJ(orig)
	if err != nil {
		t.Fatal(err)
	}
	if safe == orig || filepath.Dir(safe) != dir {
		t.Fatalf("copy at %q, want a new file in %q", safe, dir)
	}
	cleanup()
	if _, err := os.Stat(safe); !os.IsNotExist(err) {
		t.Fatal("cleanup should remove the copy")
	}
}
