package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// launchMode selects how the new process gets (or does not get) a window.
type launchMode int

const (
	// launchVisible starts the program normally.
	launchVisible launchMode = iota
	// launchNoWindow starts the program without any console window
	// (CREATE_NO_WINDOW); used for scripts and "launch hidden" entries.
	launchNoWindow
	// launchHiddenConsole starts a console program with its console window
	// created hidden (SW_HIDE). The window still exists, so it can be shown
	// again later from a tray icon.
	launchHiddenConsole
)

// The native console path cannot use exec.Cmd.Start: Go explicitly supplies
// standard handles even when they are nil, replacing console buffers with NUL.
// Keep only the process and wait operations used by the orchestrator/callers.
type startedProcess struct {
	Process *os.Process
	wait    func() error
}

func (p *startedProcess) Wait() error { return p.wait() }

func startProcess(exePath, args string, mode launchMode) (*startedProcess, error) {
	hidden := mode == launchNoWindow
	dir := filepath.Dir(exePath)
	cmd := buildLaunchCommand(exePath, args, hidden)
	if _, err := os.Stat(dir); err == nil {
		cmd.Dir = dir
	}
	if cmd.Err != nil {
		return nil, cmd.Err
	}
	if mode == launchHiddenConsole || mode == launchVisible && isConsoleExecutable(cmd.Path) {
		return startConsoleProcess(cmd, mode == launchHiddenConsole)
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &startedProcess{Process: cmd.Process, wait: cmd.Wait}, nil
}

const (
	createNoWindow = 0x08000000
)

func buildLaunchCommand(exePath, args string, hidden bool) *exec.Cmd {
	trimmedArgs := strings.TrimSpace(args)
	if runtime.GOOS == "windows" {
		if isPythonScript(exePath) {
			return buildPythonCommand(exePath, trimmedArgs, hidden)
		}

		if isPowerShellScript(exePath) {
			return buildPowerShellCommand(exePath, trimmedArgs, hidden)
		}

		if isCmdScript(exePath) {
			cleanPath := strings.Trim(strings.TrimSpace(exePath), "\"")
			// /s removes exactly the outer quote pair; the inner pair keeps
			// script paths containing spaces quoted even with quoted arguments.
			// /d prevents cmd's AutoRun registry commands from altering startup.
			commandLine := fmt.Sprintf("cmd.exe /d /s /c \"\"%s\"", cleanPath)
			if trimmedArgs != "" {
				commandLine += " " + trimmedArgs
			}
			commandLine += "\""
			cmd := exec.Command("cmd.exe")
			cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: commandLine}
			if hidden {
				cmd.SysProcAttr.CreationFlags = createNoWindow
				cmd.SysProcAttr.HideWindow = true
			}
			return cmd
		}
	}
	if trimmedArgs == "" {
		cmd := exec.Command(exePath)
		if runtime.GOOS == "windows" && hidden {
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
		}
		return cmd
	}
	cmd := exec.Command(exePath, parseArgs(trimmedArgs)...)
	if runtime.GOOS == "windows" && hidden {
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
	}
	return cmd
}

// hasNativeConsole follows WinTray's script/interpreter rules. pythonw keeps
// GUI semantics; never allocate a fake console to recover discarded output.
func hasNativeConsole(path string) bool {
	return isCmdScript(path) || isPowerShellScript(path) || strings.EqualFold(filepath.Ext(path), ".py") || consoleExecutableCheck(path)
}

func isCmdScript(exePath string) bool {
	ext := strings.ToLower(filepath.Ext(exePath))
	return ext == ".bat" || ext == ".cmd"
}

func isPowerShellScript(exePath string) bool {
	ext := strings.ToLower(filepath.Ext(exePath))
	return ext == ".ps1"
}

func isPythonScript(exePath string) bool {
	ext := strings.ToLower(filepath.Ext(exePath))
	return ext == ".py" || ext == ".pyw"
}

func buildPythonCommand(scriptPath, args string, hidden bool) *exec.Cmd {
	cleanPath := strings.Trim(strings.TrimSpace(scriptPath), "\"")
	cmdArgs := []string{cleanPath}
	if args != "" {
		cmdArgs = append(cmdArgs, parseArgs(args)...)
	}
	launcher := "python.exe"
	if strings.EqualFold(filepath.Ext(cleanPath), ".pyw") {
		launcher = "pythonw.exe"
	}
	cmd := exec.Command(launcher, cmdArgs...)
	if hidden {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: createNoWindow,
			HideWindow:    true,
		}
	}
	return cmd
}

func buildPowerShellCommand(scriptPath, args string, hidden bool) *exec.Cmd {
	cleanPath := strings.Trim(strings.TrimSpace(scriptPath), "\"")
	cmdArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", cleanPath}
	if args != "" {
		cmdArgs = append(cmdArgs, parseArgs(args)...)
	}
	cmd := exec.Command("powershell.exe", cmdArgs...)
	if hidden {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: createNoWindow,
			HideWindow:    true,
		}
	}
	return cmd
}

// parseArgs follows Windows command-line quoting rules, including empty
// arguments and backslashes immediately before double quotes.
func parseArgs(s string) []string {
	var args []string
	for i := 0; i < len(s); {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i == len(s) {
			break
		}
		var arg strings.Builder
		inQuote := false
		for i < len(s) && (inQuote || (s[i] != ' ' && s[i] != '\t')) {
			slashes := 0
			for i < len(s) && s[i] == '\\' {
				slashes++
				i++
			}
			if i < len(s) && s[i] == '"' {
				arg.WriteString(strings.Repeat(`\`, slashes/2))
				if slashes%2 != 0 {
					arg.WriteByte('"')
				} else if inQuote && i+1 < len(s) && s[i+1] == '"' {
					arg.WriteByte('"')
					i++
				} else {
					inQuote = !inQuote
				}
				i++
				continue
			}
			arg.WriteString(strings.Repeat(`\`, slashes))
			if i == len(s) || (!inQuote && (s[i] == ' ' || s[i] == '\t')) {
				break
			}
			arg.WriteByte(s[i])
			i++
		}
		args = append(args, arg.String())
	}
	return args
}
