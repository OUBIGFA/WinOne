//go:build windows

package ai

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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
		privateAgent := strings.HasPrefix(name, "WINONE_AI_PRIVACY_") || name == "WINONE_AI_USAGE_PATH" || name == "WINONE_AI_PLUGINS_HOME" || name == "WINONE_AI_TRANSPORT_ONLY" || name == "GH_TOKEN" || name == "GITHUB_TOKEN" || name == "GOOGLE_APPLICATION_CREDENTIALS"
		for _, prefix := range []string{"CODEX_", "CLAUDE_", "ANTHROPIC_", "OPENAI_", "GEMINI_", "CLOUDSDK_", "COPILOT_", "CURSOR_", "GROK_", "KIRO_", "DEVIN_", "WORKBUDDY_", "QODER_", "ZED_", "FACTORY_", "DROID_", "MIMO_", "ZCODE_", "AWS_", "AZURE_"} {
			privateAgent = privateAgent || strings.HasPrefix(name, prefix)
		}
		if privateAgent {
			continue
		}
		if strings.HasPrefix(name, "XDG_") || strings.HasPrefix(name, "MAGPIE_") || name == "USERPROFILE" || name == "APPDATA" || name == "LOCALAPPDATA" || name == "CLAUDE_CONFIG_DIR" || name == "CLAUDE_SECURESTORAGE_CONFIG_DIR" || name == "ANTHROPIC_API_KEY" || name == "OPENAI_API_KEY" {
			continue
		}
		env = append(env, value)
	}
	// Only the child sees this private agent home. Login never switches the
	// user's separately installed coding agents to another account.
	return append(env, "XDG_CONFIG_HOME="+filepath.Join(directory, "config"), "XDG_CACHE_HOME="+filepath.Join(directory, "cache"), "USERPROFILE="+filepath.Join(directory, "home"), "APPDATA="+filepath.Join(directory, "home/AppData/Roaming"), "LOCALAPPDATA="+filepath.Join(directory, "home/AppData/Local"), "MAGPIE_OTEL_ENABLED=false")
}
