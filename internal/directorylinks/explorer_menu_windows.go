//go:build windows

package directorylinks

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const menuOwner = "WinOne.DirectoryLinks.v1"
const classesRoot = `Software\Classes`

var menuEntries = []string{`*\shell\WinOne.DirectoryLinks`, `Directory\shell\WinOne.DirectoryLinks`}

type ExplorerMenuState struct {
	Enabled  bool   `json:"enabled"`
	Partial  bool   `json:"partial"`
	Conflict string `json:"conflict,omitempty"`
}

func menuCommand(executable string) string {
	return windows.ComposeCommandLine([]string{executable, "--directory-links", "--source"}) + ` "%1"`
}

func ExplorerMenu(executable string) (ExplorerMenuState, error) {
	return explorerMenu(classesRoot, executable)
}

func explorerMenu(base, executable string) (ExplorerMenuState, error) {
	var state ExplorerMenuState
	count := 0
	for _, relative := range menuEntries {
		key, err := registry.OpenKey(registry.CURRENT_USER, base+`\`+relative, registry.READ|registry.WOW64_64KEY)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			return state, err
		}
		owner, _, e := key.GetStringValue("WinOneOwner")
		path, _, pe := key.GetStringValue("WinOnePath")
		key.Close()
		if e != nil || pe != nil || owner != menuOwner || !strings.EqualFold(filepath.Clean(path), filepath.Clean(executable)) {
			state.Conflict = "已有其他安装或程序使用此菜单位置"
			continue
		}
		command, err := registry.OpenKey(registry.CURRENT_USER, base+`\`+relative+`\command`, registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err != nil {
			state.Partial = true
			continue
		}
		value, _, err := command.GetStringValue("")
		command.Close()
		if err != nil || value != menuCommand(executable) {
			state.Conflict = "菜单命令已被修改，请先保留现有设置"
			continue
		}
		count++
	}
	state.Enabled = count == len(menuEntries) && state.Conflict == ""
	state.Partial = state.Partial || count > 0 && !state.Enabled
	return state, nil
}

// ConfigureExplorerMenu preserves mklink's file and directory verbs, scoped to
// this user. It never writes HKLM or overwrites another installation's command.
func ConfigureExplorerMenu(executable string, enabled bool) (ExplorerMenuState, error) {
	state, err := configureExplorerMenu(classesRoot, executable, enabled)
	if err == nil {
		windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify").Call(0x08000000, 0, 0, 0)
	}
	return state, err
}

func configureExplorerMenu(base, executable string, enabled bool) (ExplorerMenuState, error) {
	if !filepath.IsAbs(executable) || !strings.EqualFold(filepath.Base(executable), "WinOne.exe") {
		return ExplorerMenuState{}, errors.New("右键入口必须使用当前 WinOne 桌面程序")
	}
	state, err := explorerMenu(base, executable)
	if err != nil {
		return state, err
	}
	if state.Conflict != "" {
		return state, errors.New(state.Conflict)
	}
	for _, relative := range menuEntries {
		path := base + `\` + relative
		if enabled {
			key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE|registry.WOW64_64KEY)
			if err != nil {
				return state, err
			}
			label := "Create link with WinOne"
			language, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
			if language&0x3ff == 4 {
				label = "使用 WinOne 创建链接"
			}
			for name, value := range map[string]string{"": label, "Icon": `"` + executable + `",0`, "WinOneOwner": menuOwner, "WinOnePath": executable} {
				if err = key.SetStringValue(name, value); err != nil {
					key.Close()
					return state, err
				}
			}
			key.Close()
			command, _, err := registry.CreateKey(registry.CURRENT_USER, path+`\command`, registry.SET_VALUE|registry.WOW64_64KEY)
			if err != nil {
				return state, err
			}
			err = command.SetStringValue("", menuCommand(executable))
			command.Close()
			if err != nil {
				return state, err
			}
		} else {
			key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.READ|registry.WOW64_64KEY)
			if errors.Is(err, registry.ErrNotExist) {
				continue
			}
			if err != nil {
				return state, err
			}
			children, err := key.ReadSubKeyNames(-1)
			key.Close()
			if err != nil {
				return state, err
			}
			for _, name := range children {
				if name != "command" {
					return state, fmt.Errorf("菜单包含未识别子项，已保留：%s", name)
				}
			}
			if err = registry.DeleteKey(registry.CURRENT_USER, path+`\command`); err != nil && !errors.Is(err, registry.ErrNotExist) {
				return state, err
			}
			if err = registry.DeleteKey(registry.CURRENT_USER, path); err != nil {
				return state, err
			}
		}
	}
	return explorerMenu(base, executable)
}

// RemoveExplorerMenuOwnedBy is used by normal uninstall preparation. A menu
// owned by another installation remains intact, as do unrelated shell verbs.
func RemoveExplorerMenuOwnedBy(executable string) error {
	state, err := ExplorerMenu(executable)
	if err != nil {
		return err
	}
	if state.Conflict != "" {
		return nil
	}
	_, err = ConfigureExplorerMenu(executable, false)
	return err
}
