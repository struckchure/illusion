package asset

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
)

// Settings is a resource that configures asset loading.
type Settings struct {
	// Root is the directory asset paths are relative to. Defaults to "assets".
	Root string
}

// Plugin sets where assets are loaded from. Loaders work without it, using
// the default root.
type Plugin struct {
	// Root is the directory asset paths are relative to. Defaults to "assets".
	Root string
}

// Build implements [illusion.Plugin].
func (p Plugin) Build(app *illusion.App) {
	root := p.Root
	if root == "" {
		root = "assets"
	}
	app.InsertResource(illusion.R(&Settings{Root: root}))
}

// LoadFunc reads the file at path into an asset.
type LoadFunc[T any] func(path string) (T, error)

type loaderState[T any] struct {
	load    LoadFunc[T]
	byPath  map[string]Handle[T]
	entries map[Handle[T]]*loaded
}

type loaded struct {
	path string
	refs int
}

// RegisterLoader makes files loadable as T through [Loader]. It also
// registers the Assets[T] store, with onRemove as its release function.
func RegisterLoader[T any](app *illusion.App, load LoadFunc[T], onRemove func(*T)) {
	Register(app, onRemove)
	app.InsertResource(illusion.R(&loaderState[T]{
		load:    load,
		byPath:  map[string]Handle[T]{},
		entries: map[Handle[T]]*loaded{},
	}))
}

// Loader is a system parameter that loads files as assets of type T.
//
// Loads are reference counted: loading a path that is already loaded returns
// the same handle and adds a reference, and [Loader.Release] drops one. The
// asset is freed when the last reference is released, or at shutdown.
//
// Loading happens immediately, on the main thread, so GPU assets can be
// uploaded right away.
type Loader[T any] struct {
	state    *loaderState[T]
	assets   *Assets[T]
	settings ecs.Resource[Settings]
}

// InitParam implements [illusion.Param].
func (l *Loader[T]) InitParam(w *ecs.World) {
	l.state = ecs.GetResource[loaderState[T]](w)
	l.assets = ecs.GetResource[Assets[T]](w)
	if l.state == nil || l.assets == nil {
		var zero T
		panic(fmt.Sprintf("asset: no loader registered for %T; add the plugin that provides it", zero))
	}
	l.settings = ecs.NewResource[Settings](w)
}

// Load loads the file at path (relative to the asset root) and returns a
// handle to it.
func (l *Loader[T]) Load(path string) (Handle[T], error) {
	path = filepath.Clean(path) // "./a.png" and "a.png" are the same asset
	if h, ok := l.state.byPath[path]; ok {
		if l.assets.Contains(h) {
			l.state.entries[h].refs++
			return h, nil
		}
		// Removed from the store directly; forget it and load again.
		delete(l.state.entries, h)
		delete(l.state.byPath, path)
	}

	full := l.resolve(path)
	if _, err := os.Stat(full); err != nil {
		return Handle[T]{}, fmt.Errorf("asset: load %s: %w", path, err)
	}
	value, err := l.state.load(full)
	if err != nil {
		return Handle[T]{}, fmt.Errorf("asset: load %s: %w", path, err)
	}
	h := l.assets.Add(value)
	l.state.byPath[path] = h
	l.state.entries[h] = &loaded{path: path, refs: 1}
	return h, nil
}

// MustLoad is like Load but panics if the file can't be loaded.
func (l *Loader[T]) MustLoad(path string) Handle[T] {
	h, err := l.Load(path)
	if err != nil {
		panic(err)
	}
	return h
}

// Release drops one reference to h, freeing the asset when none are left.
// Releasing a handle that wasn't loaded from a file does nothing.
func (l *Loader[T]) Release(h Handle[T]) {
	entry, ok := l.state.entries[h]
	if !ok {
		return
	}
	entry.refs--
	if entry.refs > 0 && l.assets.Contains(h) {
		return
	}
	delete(l.state.entries, h)
	// The path may have been loaded again under a new handle since h was
	// removed from the store directly; leave that one alone.
	if l.state.byPath[entry.path] == h {
		delete(l.state.byPath, entry.path)
	}
	l.assets.Remove(h)
}

// Path returns the path h was loaded from.
func (l *Loader[T]) Path(h Handle[T]) (string, bool) {
	entry, ok := l.state.entries[h]
	if !ok {
		return "", false
	}
	return entry.path, true
}

// Get returns the asset h refers to, or nil.
func (l *Loader[T]) Get(h Handle[T]) *T {
	return l.assets.Get(h)
}

func (l *Loader[T]) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	root := "assets"
	if s := l.settings.Get(); s != nil {
		root = s.Root
	}
	return filepath.Join(root, path)
}
