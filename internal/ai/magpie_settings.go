package ai

import (
	"errors"
	"slices"
	"time"
	_ "time/tzdata"
)

// Retained account execution, privacy and billing-time settings. Removed
// management pages cannot enable services through an old preferences document.
type MagpieSettings struct {
	ClaudePrintMode     bool     `json:"claudePrintMode,omitempty"`
	ClaudeNoExtraUsage  bool     `json:"claudeNoExtraUsage,omitempty"`
	CodexClient         string   `json:"codexClient,omitempty"`
	CodexResetDaysLeast int      `json:"codexResetDaysLeast,omitempty"`
	CodexResetDaysMost  int      `json:"codexResetDaysMost,omitempty"`
	ClaudeWaysOut       []string `json:"claudeWaysOut,omitempty"`
	ClaudeRegionHold    bool     `json:"claudeRegionHold,omitempty"`
	CodexAutoReset      []string `json:"codexAutoReset,omitempty"`
	TimeZone            string   `json:"timeZone,omitempty"`
	HideRequestText     bool     `json:"hideRequestText,omitempty"`
	// The public gateway fills these from its machine-local settings. They
	// are not copied from portable AI provider preferences.
	BareModelIDs     bool `json:"bareModelIDs,omitempty"`
	BareModelIDsOnly bool `json:"bareModelIDsOnly,omitempty"`
	NoStats          bool `json:"noStats"`
	NoUsageStats     bool `json:"noUsageStats"`
}

func (s *MagpieSettings) clone() *MagpieSettings {
	out := &MagpieSettings{NoStats: true, NoUsageStats: true}
	if s != nil {
		out.ClaudePrintMode = s.ClaudePrintMode
		out.ClaudeNoExtraUsage = s.ClaudeNoExtraUsage
		out.CodexClient = s.CodexClient
		out.CodexResetDaysLeast, out.CodexResetDaysMost = s.CodexResetDaysLeast, s.CodexResetDaysMost
		out.ClaudeWaysOut = slices.Clone(s.ClaudeWaysOut)
		out.CodexAutoReset = slices.Clone(s.CodexAutoReset)
		out.ClaudeRegionHold = s.ClaudeRegionHold
		out.TimeZone, out.HideRequestText = s.TimeZone, s.HideRequestText
	}
	return out
}
func (s *MagpieSettings) validate() error {
	if s == nil {
		return nil
	}
	if s.CodexResetDaysLeast < 0 || s.CodexResetDaysLeast > 365 || s.CodexResetDaysMost < 0 || s.CodexResetDaysMost > 365 {
		return errors.New("Codex 重置剩余天数须为 0–365")
	}
	if s.CodexResetDaysMost > 0 && s.CodexResetDaysLeast > s.CodexResetDaysMost {
		return errors.New("最少剩余天数不能超过最多剩余天数")
	}
	if s.TimeZone != "" {
		if _, err := time.LoadLocation(s.TimeZone); err != nil || s.TimeZone == "Local" {
			return errors.New("请输入有效的时区，例如 Asia/Shanghai 或 UTC")
		}
	}
	for _, w := range s.ClaudeWaysOut {
		if !MagpieWayOut(w) {
			return errors.New("Claude 出口须为地区代码、IP 或 CIDR 网段")
		}
	}
	if s != nil && s.CodexClient != "" && s.CodexClient != "desktop" {
		return errors.New("Codex 请求来源必须为 CLI 或桌面应用")
	}
	return nil
}
