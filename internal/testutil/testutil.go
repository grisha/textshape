// Package testutil provides utilities for testing.
package testutil

import (
	"os"
	"path/filepath"
	"runtime"
)

// FindTestFont locates a test font by name.
// It searches in the testdata/fonts directory relative to the go-hb module root.
func FindTestFont(name string) string {
	// Get the directory of this source file
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}

	// Navigate from internal/testutil to the module root
	moduleRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	// Primary location: testdata/fonts
	primary := filepath.Join(moduleRoot, "testdata", "fonts", name)
	if _, err := os.Stat(primary); err == nil {
		return primary
	}

	// Fallback locations for backwards compatibility
	fallbacks := []string{
		filepath.Join(moduleRoot, "testdata", name),
		filepath.Join(moduleRoot, "ot", "testdata", name),
		filepath.Join(moduleRoot, "subset", "testdata", name),
	}

	for _, p := range fallbacks {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// MustFindTestFont is like FindTestFont but panics if the font is not found.
func MustFindTestFont(name string) string {
	path := FindTestFont(name)
	if path == "" {
		panic("test font not found: " + name)
	}
	return path
}
