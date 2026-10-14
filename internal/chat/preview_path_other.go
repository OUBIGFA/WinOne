//go:build !windows

package chat

import "path/filepath"

func resolvePreviewPath(path string) (string, error) { return filepath.EvalSymlinks(path) }
