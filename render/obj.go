package render

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// raylib's OBJ parser (tinyobj) aborts the whole process on a face with more
// than five vertices, instead of returning an error. Blender exports those
// for flat n-gons, so such files are rewritten with the big faces split into
// triangle fans (which is how tinyobj triangulates smaller faces anyway).
const maxOBJFaceVertices = 5

// sanitizeOBJ returns a path raylib can load safely: path itself, or a
// temporary rewritten copy that cleanup removes. The copy goes next to the
// original, because raylib loads a material's textures relative to the OBJ's
// directory; only if that directory isn't writable does it go to the system
// temp directory (textures named by relative paths in a .mtl then can't load).
func sanitizeOBJ(path string) (safe string, cleanup func(), err error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	fixed, changed := triangulateOBJ(src, filepath.Dir(path))
	if !changed {
		return path, func() {}, nil
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".illusion-*.obj")
	if err != nil {
		if f, err = os.CreateTemp("", "illusion-*.obj"); err != nil {
			return "", nil, err
		}
	}
	defer f.Close()
	if _, err := f.Write(fixed); err != nil {
		os.Remove(f.Name())
		return "", nil, err
	}
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

// triangulateOBJ splits faces with more than maxOBJFaceVertices vertices into
// fans. Because the result is loaded from elsewhere, relative mtllib paths
// are made absolute against dir. changed is false if nothing needed fixing.
func triangulateOBJ(src []byte, dir string) (out []byte, changed bool) {
	var buf bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(src))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var mtllibs []string
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		switch {
		case len(fields) > 1+maxOBJFaceVertices && fields[0] == "f":
			changed = true
			verts := fields[1:]
			for i := 1; i+1 < len(verts); i++ {
				buf.WriteString("f " + verts[0] + " " + verts[i] + " " + verts[i+1] + "\n")
			}
			continue
		case len(fields) > 1 && fields[0] == "mtllib":
			mtllibs = append(mtllibs, line)
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if !changed {
		return src, false
	}
	for _, line := range mtllibs {
		name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "mtllib"))
		if !filepath.IsAbs(name) {
			fixedLine := "mtllib " + filepath.Join(dir, name)
			out := bytes.Replace(buf.Bytes(), []byte(line+"\n"), []byte(fixedLine+"\n"), 1)
			buf.Reset()
			buf.Write(out)
		}
	}
	return buf.Bytes(), true
}
