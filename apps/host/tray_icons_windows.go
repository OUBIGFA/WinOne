//go:build windows

package main

import (
	"errors"
	"fmt"
	"winone/internal/branding"
)

type trayTemplateIcon interface {
	SetTemplateIconPNG([]byte) error
}

// Reuse the native menu's owned PNG lifetime and theme snapshot. An icon
// failure is reported without removing either indispensable menu action.
func setTrayMenuIcons(open, quit trayTemplateIcon) error {
	var failures []error
	for _, item := range []struct {
		view trayTemplateIcon
		name string
		data []byte
	}{{open, "打开 WinOne", branding.TrayOpenIcon}, {quit, "退出 WinOne", branding.TrayExitIcon}} {
		if err := item.view.SetTemplateIconPNG(item.data); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", item.name, err))
		}
	}
	return errors.Join(failures...)
}
