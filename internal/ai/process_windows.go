//go:build windows

package ai

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"winone/internal/desktopparent"
)

func hideConsole(command *exec.Cmd) {
	// Match Magpie's proc.hide: suppress the worker's console only. SW_HIDE
	// also changes the initial ShowWindow contract for native login windows.
	// A plugin's dedicated GUI browser must remain visible to finish sign-in.
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}

// A browser must not inherit the elevated Host token: Chromium relaunches
// itself unelevated, outside the plugin's startup/Job lifetime. Start this AI
// worker with the same user's limited token instead; never disable its sandbox.
func limitWorkerPrivilege(command *exec.Cmd) (func(), error) {
	current := windows.GetCurrentProcessToken()
	if !current.IsElevated() {
		return func() {}, nil
	}
	// Chromium's documented Windows de-elevation starts under Explorer's
	// process context. Use the same parent-process contract without an untracked
	// browser relaunch. Verify the shell token's SID/elevation before adoption.
	parent, err := desktopparent.Open()
	switch {
	case errors.Is(err, desktopparent.ErrDesktopUnavailable):
		return nil, errors.New("Windows 桌面尚未就绪，无法启动普通权限 AI 服务")
	case errors.Is(err, desktopparent.ErrDifferentAccount):
		return nil, errors.New("无法确认 AI 浏览器的 Windows 账户")
	case err != nil:
		return nil, err
	}
	if parent.Elevated {
		parent.Close()
		return nil, errors.New("AI 浏览器运行环境仍为管理员权限")
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	parent.Apply(command.SysProcAttr)
	return parent.Close, nil
}

func isolatedEnvironment(directory string) []string {
	var env []string
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		name = strings.ToUpper(name)
		// Preserve an explicit process-wide telemetry opt-out. Ordinary workers
		// use the saved privacy preference, whose WinOne default is off.
		if name == "MAGPIE_NO_STATS" {
			env = append(env, value)
			continue
		}
		privateAgent := strings.HasPrefix(name, "WINONE_AI_PRIVACY_") || strings.HasPrefix(name, "WINONE_AI_EXTERNAL_") || name == "WINONE_AI_USAGE_PATH" || name == "WINONE_AI_PLUGINS_HOME" || name == "WINONE_AI_TRANSPORT_ONLY" || name == "GH_TOKEN" || name == "GITHUB_TOKEN" || name == "GOOGLE_APPLICATION_CREDENTIALS"
		for _, prefix := range []string{"CODEX_", "CLAUDE_", "ANTHROPIC_", "OPENAI_", "GEMINI_", "CLOUDSDK_", "COPILOT_", "CURSOR_", "GROK_", "KIRO_", "DEVIN_", "WORKBUDDY_", "QODER_", "ZED_", "FACTORY_", "DROID_", "MIMO_", "ZCODE_", "AWS_", "AZURE_"} {
			privateAgent = privateAgent || strings.HasPrefix(name, prefix)
		}
		if privateAgent || name == "WINONE_AI_CLAUDE_RUN_FILE" || name == "WINONE_AI_OS_USERPROFILE" || name == "WINONE_AI_BACKGROUND_DESKTOP" {
			continue
		}
		if strings.HasPrefix(name, "XDG_") || strings.HasPrefix(name, "MAGPIE_") || name == "USERPROFILE" || name == "APPDATA" || name == "LOCALAPPDATA" || name == "CLAUDE_CONFIG_DIR" || name == "CLAUDE_SECURESTORAGE_CONFIG_DIR" || name == "ANTHROPIC_API_KEY" || name == "OPENAI_API_KEY" {
			continue
		}
		env = append(env, value)
	}
	// Only the child sees this private agent home. Login never switches the
	// user's separately installed coding agents to another account.
	// Inherited MAGPIE_* overrides were removed above. Export follows the
	// installation's saved opt-in; a forced false here made its switch inert.
	return append(env, "XDG_CONFIG_HOME="+filepath.Join(directory, "config"), "XDG_CACHE_HOME="+filepath.Join(directory, "cache"), "USERPROFILE="+filepath.Join(directory, "home"), "APPDATA="+filepath.Join(directory, "home/AppData/Roaming"), "LOCALAPPDATA="+filepath.Join(directory, "home/AppData/Local"))
}
