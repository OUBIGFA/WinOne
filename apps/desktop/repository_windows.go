//go:build windows

package main

import (
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Replace this sample destination when the project has its published repository.
const projectRepositoryURL = "https://github.com/example/WinOne"

func (a *App) OpenRepository() error {
	ctx := a.windowContext()
	if ctx == nil {
		return errors.New(a.dialogText("窗口尚未就绪", "Settings window is not ready"))
	}
	runtime.BrowserOpenURL(ctx, projectRepositoryURL)
	return nil
}
