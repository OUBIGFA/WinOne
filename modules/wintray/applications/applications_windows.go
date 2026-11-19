//go:build windows

package applications

import (
	"context"
	"image"
	"winone/modules/wintray/internal/shellapp"
)

func Enumerate(ctx context.Context) ([]Application, error) { return shellapp.Enumerate(ctx) }
func Icon(path string, size int) (*image.RGBA, error)      { return shellapp.Icon(path, size) }
func InstallationDirectory(path string) (string, error)    { return shellapp.InstallationDirectory(path) }
