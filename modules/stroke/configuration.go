// Package stroke defines the Host-owned settings for the embedded Stroke engine.
package stroke

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

//go:embed defaults.json
var defaultJSON []byte

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}
type Gesture struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Points  []Point `json:"points,omitempty"`
	Vectors [][]int `json:"vectors,omitempty"`
}
type Action struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Gesture string `json:"gesture"`
	Code    string `json:"code"`
	Enabled bool   `json:"enabled"`
}
type ActionPackage struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Actions []Action `json:"actions"`
}
type Scope struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Enabled       bool     `json:"enabled"`
	ProcessPath   string   `json:"processPath,omitempty"`
	Pattern       string   `json:"pattern,omitempty"`
	WindowClasses []string `json:"windowClasses,omitempty"`
	Packages      []string `json:"packages"`
}
type Pen struct {
	Enabled   *bool   `json:"enabled,omitempty"`
	Color     string  `json:"color"`
	Opacity   float64 `json:"opacity"`
	Thickness int     `json:"thickness"`
}

// Missing in profiles saved before the switch existed means enabled.
func (p Pen) IsEnabled() bool { return p.Enabled == nil || *p.Enabled }

type Preferences struct {
	Version              int             `json:"version"`
	Revision             uint64          `json:"revision,omitempty"`
	DefaultPolicy        string          `json:"defaultPolicy"`
	TriggerButton        string          `json:"triggerButton"`
	Pen                  Pen             `json:"pen"`
	Gestures             []Gesture       `json:"gestures"`
	ActionPackages       []ActionPackage `json:"actionPackages"`
	Scopes               []Scope         `json:"scopes"`
	Filters              []string        `json:"filters"`
	ScriptThreadCount    int             `json:"scriptThreadCount"`
	ScriptTimeoutSeconds int             `json:"scriptTimeoutSeconds"`
	Namespaces           []string        `json:"namespaces"`
	Assemblies           []string        `json:"assemblies"`
}

// Defaults are applied only for a missing Host namespace. They never overwrite
// an existing disabled scope or a user's gesture/action changes during reload.
func Defaults() Preferences {
	var p Preferences
	if err := json.Unmarshal(defaultJSON, &p); err != nil {
		panic(err)
	}
	return p
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validName(name string) bool {
	return strings.TrimSpace(name) != "" && len(name) <= 240 && !strings.ContainsAny(name, "\r\n\x00")
}

func (p Preferences) Validate() error {
	if p.Version != 1 || p.DefaultPolicy != "disabled" {
		return errors.New("鼠标手势配置版本或默认范围无效")
	}
	if !slices.Contains([]string{"right", "middle", "x1", "x2", "left"}, p.TriggerButton) {
		return errors.New("触发按键无效")
	}
	if !colorPattern.MatchString(p.Pen.Color) || p.Pen.Opacity < 0 || p.Pen.Opacity > 1 || p.Pen.Thickness < 0 || p.Pen.Thickness > 10 {
		return errors.New("轨迹颜色、透明度或宽度无效")
	}
	if p.ScriptThreadCount < 1 || p.ScriptThreadCount > 64 || p.ScriptTimeoutSeconds < 0 || p.ScriptTimeoutSeconds > 3600 {
		return errors.New("执行并发须为 1–64，动作超时须为 0–3600 秒；0 不限制时长")
	}
	if len(p.Gestures) > 256 || len(p.ActionPackages) > 32 || len(p.Scopes) > 64 || len(p.Filters) > 32 || len(p.Namespaces) > 128 || len(p.Assemblies) > 64 {
		return errors.New("手势配置项目过多")
	}
	gestures := map[string]bool{}
	for _, g := range p.Gestures {
		if !idPattern.MatchString(g.ID) || gestures[g.ID] || !validName(g.Name) {
			return errors.New("手势名称或标识无效")
		}
		gestures[g.ID] = true
		if len(g.Vectors) != 0 && len(g.Vectors) != 128 {
			return errors.New("Stroke 手势必须包含 128 个方向向量")
		}
		for _, v := range g.Vectors {
			if len(v) != 2 || v[0] < -127 || v[0] > 127 || v[1] < -127 || v[1] > 127 || v[0] == 0 && v[1] == 0 {
				return errors.New("手势向量无效")
			}
		}
		if len(g.Points) > 4096 || len(g.Points) == 1 {
			return errors.New("请录制有效手势轨迹")
		}
		for _, point := range g.Points {
			if point.X < -100000 || point.X > 100000 || point.Y < -100000 || point.Y > 100000 {
				return errors.New("轨迹坐标超出范围")
			}
		}
	}
	packages := map[string]bool{}
	for _, pack := range p.ActionPackages {
		if !idPattern.MatchString(pack.ID) || packages[pack.ID] || !validName(pack.Name) || len(pack.Actions) > 256 {
			return errors.New("动作包名称或标识无效")
		}
		packages[pack.ID] = true
		actions := map[string]bool{}
		for _, a := range pack.Actions {
			if !idPattern.MatchString(a.ID) || actions[a.ID] || !validName(a.Name) || len(a.Code) > 32000 || strings.ContainsRune(a.Code, 0) {
				return errors.New("动作名称、脚本或标识无效")
			}
			actions[a.ID] = true
			if !gestures[a.Gesture] && !slices.Contains([]string{"#0", "#1", "#2", "#3", "#4", "#5", "#6"}, a.Gesture) {
				return fmt.Errorf("%s 引用了未知手势", a.Name)
			}
		}
	}
	scopes := map[string]bool{}
	for _, s := range p.Scopes {
		if !idPattern.MatchString(s.ID) || scopes[s.ID] || !validName(s.Name) || !slices.Contains([]string{"explorer-folders", "application", "global"}, s.Kind) {
			return errors.New("应用范围无效")
		}
		scopes[s.ID] = true
		if s.Kind == "application" && strings.TrimSpace(s.ProcessPath) == "" && strings.TrimSpace(s.Pattern) == "" {
			return errors.New("应用范围需要程序路径或匹配规则")
		}
		if len(s.ProcessPath) > 32767 || len(s.Pattern) > 8192 || len(s.WindowClasses) > 16 {
			return errors.New("应用范围过长")
		}
		if s.Pattern != "" {
			// Stroke uses .NET regular expressions (including lookarounds).
			// Syntax is checked by its runtime; Go's RE2 is not equivalent.
			for _, pattern := range strings.Split(strings.ReplaceAll(s.Pattern, "\r", ""), "\n") {
				if len(pattern) > 512 || strings.ContainsRune(pattern, 0) {
					return errors.New("应用匹配规则过长或包含无效字符")
				}
			}
		}
		for _, name := range s.WindowClasses {
			if !validName(name) {
				return errors.New("窗口类名无效")
			}
		}
		for _, id := range s.Packages {
			if !packages[id] {
				return errors.New("应用范围引用了未知动作包")
			}
		}
	}
	for _, filter := range p.Filters {
		if len(filter) > 512 || strings.ContainsRune(filter, 0) {
			return errors.New("排除规则过长")
		}
	}
	for _, name := range p.Namespaces {
		if strings.TrimSpace(name) == "" || len(name) > 512 || strings.ContainsAny(name, "\r\n\x00") {
			return errors.New("脚本命名空间无效")
		}
	}
	for _, assembly := range p.Assemblies {
		if !regexp.MustCompile(`(?i)^[A-Za-z0-9_.-]+[.]dll$`).MatchString(assembly) {
			if len(assembly) > 32767 || !regexp.MustCompile(`(?i)^[A-Za-z]:[\\/].+[.]dll$`).MatchString(assembly) || strings.ContainsAny(assembly, "\r\n\x00") {
				return errors.New("自定义程序集需要本机 DLL 的完整路径；Stroke.Base.dll 自动提供")
			}
		}
	}
	return nil
}
