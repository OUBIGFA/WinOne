// Package app is the WinOne.exe process: its startup arguments, the
// maintenance lease and administrator handoff, the single settings window
// per Host, and that window's lifetime. It keeps the contracts the Host,
// the installer and Explorer's menu entries already use with the previous
// desktop (apps/desktop-rust): the same arguments, window identity names
// and messages.
package app

import (
	"errors"
	"path/filepath"
	"strings"
)

// Options are the parsed startup arguments.
type Options struct {
	Pipe      string
	Directory string
	// OpenSettings asks for the window (the Host's tray click and the
	// Start menu entry pass it).
	OpenSettings bool
	// Standard skips a remembered administrator start, for recovery.
	Standard bool
	// CaptureDraft resumes a screenshot draft without a settings window.
	CaptureDraft string
	// DirectoryLinks and Interconnect open those pages with paths from
	// Explorer's menu; LinkOptions are the link entry's choices.
	DirectoryLinks bool
	Interconnect   bool
	LinkSources    []string
	LinkOptions    map[string]any
}

// ParseOptions parses args as the Rust desktop's startup::Options::parse
// did, rejecting duplicates, missing values and remote pipes.
func ParseOptions(args []string, defaultPipe, local string) (Options, error) {
	if !filepath.IsAbs(local) {
		return Options{}, errors.New("无法确定本机配置目录。")
	}
	o := Options{Pipe: defaultPipe, Directory: filepath.Join(local, "WinOne"), LinkOptions: map[string]any{}}
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		argument := args[i]
		if argument != "--source" {
			if seen[argument] {
				return Options{}, errors.New("启动参数重复。")
			}
			seen[argument] = true
		}
		switch argument {
		case "--pipe", "--data-dir", "--capture-draft", "--source", "--link-type", "--link-targets":
			i++
			if i >= len(args) || args[i] == "" || strings.HasPrefix(args[i], "--") {
				return Options{}, errors.New("启动参数缺少值。")
			}
			value := args[i]
			switch argument {
			case "--pipe":
				o.Pipe = value
			case "--capture-draft":
				o.CaptureDraft = value
			case "--source":
				o.LinkSources = append(o.LinkSources, value)
			case "--link-type":
				if value != "auto" && value != "hardlink" && value != "junction" && value != "symlink" {
					return Options{}, errors.New("链接类型无效。")
				}
				o.LinkOptions["kind"] = value
			case "--link-targets":
				if value != "absolute" && value != "relative" {
					return Options{}, errors.New("链接路径类型无效。")
				}
				o.LinkOptions["relative"] = value == "relative"
			default:
				o.Directory = value
			}
		case "--open-settings":
			o.OpenSettings = true
		case "--directory-links":
			o.DirectoryLinks, o.OpenSettings = true, true
		case "--interconnect":
			o.Interconnect, o.OpenSettings = true, true
		case "--absolute-paths":
			o.LinkOptions["relative"] = false
		case "--link-retain-script", "--retain-script":
			o.LinkOptions["retainScript"] = true
		case "--link-hide-success", "--hide-success-dialog":
			o.LinkOptions["hideSuccess"] = true
		case "--standard-session":
			o.Standard = true
		default:
			if value, ok := strings.CutPrefix(argument, "--link-targets="); ok {
				if value != "absolute" && value != "relative" {
					return Options{}, errors.New("链接路径类型无效。")
				}
				o.LinkOptions["relative"] = value == "relative"
				continue
			}
			return Options{}, errors.New("不支持的启动参数。")
		}
	}
	if !strings.HasPrefix(o.Pipe, `\\.\pipe\WinOne-`) || strings.ContainsRune(o.Pipe, 0) || len(o.Pipe) > 256 ||
		!filepath.IsAbs(o.Directory) || strings.ContainsRune(o.Directory, 0) {
		return Options{}, errors.New("启动路径或管道无效。")
	}
	if o.CaptureDraft != "" && (!canonicalDraft(o.CaptureDraft) || o.OpenSettings) {
		return Options{}, errors.New("截图启动参数无效。")
	}
	if o.DirectoryLinks && o.Interconnect || (!o.DirectoryLinks && !o.Interconnect && len(o.LinkSources) > 0) ||
		(!o.DirectoryLinks && len(o.LinkOptions) > 0) || len(o.LinkSources) > 200 {
		return Options{}, errors.New("目录链接启动参数无效。")
	}
	for _, source := range o.LinkSources {
		if !filepath.IsAbs(source) || strings.ContainsRune(source, 0) {
			return Options{}, errors.New("目录链接启动参数无效。")
		}
	}
	return o, nil
}

// canonicalDraft accepts the Host's draft identifiers: 32 lowercase hex
// digits.
func canonicalDraft(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// IsProbe reports arguments of the installer's startup check.
func IsProbe(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--check-ui") || strings.HasPrefix(arg, "--probe-root") {
			return true
		}
	}
	return false
}

// wakeRequest reads the arguments a second launch forwards to the open
// window: only --open-settings, and the profile arguments that selected
// this window, are accepted.
func wakeRequest(args []string) (settings bool, ok bool) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--open-settings":
			settings = true
		case "--standard-session":
		case "--pipe", "--data-dir":
			i++
			if i >= len(args) {
				return false, false
			}
		default:
			return false, false
		}
	}
	return settings, true
}
