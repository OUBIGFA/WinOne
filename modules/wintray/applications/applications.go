// Package applications exposes WinTray's Windows application catalog to the
// unified Host and settings window without duplicating its Shell implementation.
package applications

import "winone/modules/wintray/internal/shellapp"

type Application = shellapp.Application

const PathPrefix = shellapp.PathPrefix

func IsPath(path string) bool { return shellapp.IsPath(path) }
func ID(path string) string   { return shellapp.ID(path) }
