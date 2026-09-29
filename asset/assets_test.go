package asset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
)

func TestAssets(t *testing.T) {
	var removed []string
	a := New(func(s *string) { removed = append(removed, *s) })

	h1 := a.Add("one")
	h2 := a.Add("two")
	if *a.Get(h1) != "one" || *a.Get(h2) != "two" || a.Len() != 2 {
		t.Fatal("lookup failed")
	}

	a.Remove(h1)
	if a.Get(h1) != nil || a.Contains(h1) {
		t.Fatal("removed asset still reachable")
	}

	h3 := a.Add("three") // reuses h1's slot
	if a.Get(h1) != nil || *a.Get(h3) != "three" {
		t.Fatal("stale handle reached a recycled slot")
	}

	var zero Handle[string]
	if !zero.IsZero() || a.Get(zero) != nil {
		t.Fatal("zero handle should refer to nothing")
	}

	a.Clear()
	if a.Len() != 0 || len(removed) != 3 {
		t.Fatalf("len=%d removed=%v", a.Len(), removed)
	}
}

func TestLoader(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "greeting.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	loads := 0
	var removed []string
	app := illusion.New().AddPlugins(Plugin{Root: dir})
	RegisterLoader(app, func(path string) (string, error) {
		loads++
		b, err := os.ReadFile(path)
		return string(b), err
	}, func(s *string) { removed = append(removed, *s) })

	var l Loader[string]
	l.InitParam(app.World)

	h1 := l.MustLoad("greeting.txt")
	h2 := l.MustLoad("greeting.txt")
	if h1 != h2 || loads != 1 || *l.Get(h1) != "hello" {
		t.Fatalf("expected one cached load, got %d loads", loads)
	}
	if p, _ := l.Path(h1); p != "greeting.txt" {
		t.Fatalf("path %q", p)
	}

	l.Release(h1)
	if l.Get(h1) == nil {
		t.Fatal("asset freed while a reference remains")
	}
	l.Release(h2)
	if l.Get(h1) != nil || len(removed) != 1 {
		t.Fatal("asset should be freed after the last release")
	}

	if _, err := l.Load("missing.txt"); err == nil || !strings.Contains(err.Error(), "missing.txt") {
		t.Fatalf("expected a not-found error, got %v", err)
	}

	l.MustLoad("greeting.txt")
	app.Cleanup()
	if len(removed) != 2 {
		t.Fatal("shutdown should free loaded assets")
	}
}

func TestLoaderAfterDirectRemove(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	loads := 0
	app := illusion.New().AddPlugins(Plugin{Root: dir})
	RegisterLoader(app, func(path string) (string, error) {
		loads++
		b, err := os.ReadFile(path)
		return string(b), err
	}, nil)
	var l Loader[string]
	l.InitParam(app.World)
	store := ecs.GetResource[Assets[string]](app.World)

	h1 := l.MustLoad("a.txt")
	store.Remove(h1)          // bypasses the loader
	h2 := l.MustLoad("a.txt") // reloads
	l.Release(h1)             // stale: must not disturb h2
	if h3 := l.MustLoad("a.txt"); h3 != h2 || loads != 2 {
		t.Fatalf("expected the cached h2 back (2 loads), got %v vs %v after %d loads", h3, h2, loads)
	}
}

func TestLoaderTreatsEquivalentPathsAsOneAsset(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	loads := 0
	app := illusion.New().AddPlugins(Plugin{Root: dir})
	RegisterLoader(app, func(path string) (string, error) {
		loads++
		b, err := os.ReadFile(path)
		return string(b), err
	}, nil)
	var l Loader[string]
	l.InitParam(app.World)

	h := l.MustLoad("sub/a.txt")
	for _, p := range []string{"./sub/a.txt", "sub//a.txt", "sub/../sub/a.txt"} {
		if l.MustLoad(p) != h {
			t.Fatalf("%q should resolve to the same asset as sub/a.txt", p)
		}
	}
	if loads != 1 {
		t.Fatalf("expected one load, got %d", loads)
	}
}
