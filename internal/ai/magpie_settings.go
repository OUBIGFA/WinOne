package ai

import (
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// MagpieSettings preserves upstream setting names. WinOne owns persistence;
// each worker receives one immutable snapshot, never another app's settings.
type MagpieSettings struct {
	CodexAgentsV1   bool   `json:"codexAgentsV1,omitempty"`
	FullContext     bool   `json:"fullContext,omitempty"`
	CompactAt       int    `json:"compactAt,omitempty"`
	CodexTitles     string `json:"codexTitles,omitempty"`
	CodexAutoReview string `json:"codexAutoReview,omitempty"`
	// Filled only in the private runtime snapshot for historical usage pricing.
	ModelPrices        map[string]ModelPrice `json:"modelPrices,omitempty"`
	ModelSameAs        map[string]string     `json:"modelSameAs,omitempty"`
	QuotaLeft          bool                  `json:"quotaLeft,omitempty"`
	QuotaReads         string                `json:"quotaReads,omitempty"`
	Currency           string                `json:"currency,omitempty"`
	ChineseUnits       bool                  `json:"chineseUnits,omitempty"`
	UsageAlert         int                   `json:"usageAlert,omitempty"`
	BalanceAlert       float64               `json:"balanceAlert,omitempty"`
	ResetReminder      int                   `json:"resetReminder,omitempty"`
	TrayUsages         []string              `json:"trayUsages,omitempty"`
	TrayUsageEvery     int                   `json:"trayUsageEvery,omitempty"`
	TrayNoLogos        bool                  `json:"trayNoLogos,omitempty"`
	TrayNoBird         bool                  `json:"trayNoBird,omitempty"`
	CodexWarmup        string                `json:"codexWarmup,omitempty"`
	ClaudeWarmup       string                `json:"claudeWarmup,omitempty"`
	CodexWarmAt        string                `json:"codexWarmAt,omitempty"`
	ClaudeWarmAt       string                `json:"claudeWarmAt,omitempty"`
	CodexWarmAts       []string              `json:"codexWarmAts,omitempty"`
	ClaudeWarmAts      []string              `json:"claudeWarmAts,omitempty"`
	CodexWarmAtOf      map[string]string     `json:"codexWarmAtOf,omitempty"`
	CodexAutoReset     []string              `json:"codexAutoReset,omitempty"`
	CodexNoCredits     []string              `json:"codexNoCredits,omitempty"`
	WorkBuddyCheckin   bool                  `json:"workbuddyCheckin,omitempty"`
	TraeCheckin        bool                  `json:"traeCheckin,omitempty"`
	MiniMaxCheckin     bool                  `json:"minimaxCheckin,omitempty"`
	QoderCheckin       bool                  `json:"qoderCheckin,omitempty"`
	PluginCheckins     map[string]bool       `json:"pluginCheckins,omitempty"`
	Vision             string                `json:"vision,omitempty"`
	ImageGen           string                `json:"imageGen,omitempty"`
	Searcher           string                `json:"searcher,omitempty"`
	SearchFirst        string                `json:"searchFirst,omitempty"`
	SearchAPIs         []SearchAPI           `json:"searchAPIs,omitempty"`
	MemberModel        bool                  `json:"memberModel,omitempty"`
	NoLoopGuard        bool                  `json:"noLoopGuard,omitempty"`
	PlainNames         bool                  `json:"plainNames,omitempty"`
	PlainOwnNames      bool                  `json:"plainOwnNames,omitempty"`
	Redact             bool                  `json:"redact,omitempty"`
	RedactPersonal     bool                  `json:"redactPersonal,omitempty"`
	RedactKinds        map[string]bool       `json:"redactKinds,omitempty"`
	RedactWords        []string              `json:"redactWords,omitempty"`
	RedactRules        []RedactRule          `json:"redactRules,omitempty"`
	HideAccounts       bool                  `json:"hideAccounts,omitempty"`
	NoStats            bool                  `json:"noStats"`
	NoUsageStats       bool                  `json:"noUsageStats"`
	NoAccountDiscovery bool                  `json:"noAccountDiscovery,omitempty"`
}

// Key is accepted only on save or in a private runtime snapshot. Public reads
// carry HasKey, and explicit reveal is a separate, user-triggered operation.
type SearchAPI struct {
	Vendor string `json:"vendor"`
	URL    string `json:"url,omitempty"`
	Key    string `json:"key,omitempty"`
	HasKey bool   `json:"hasKey,omitempty"`
}
type RedactRule struct {
	Kind   string `json:"kind"`
	Prefix string `json:"prefix,omitempty"`
	Regex  string `json:"regex,omitempty"`
}

func (s *MagpieSettings) clone() *MagpieSettings {
	if s == nil {
		return &MagpieSettings{NoStats: true, NoUsageStats: true}
	}
	data, _ := json.Marshal(s)
	var out MagpieSettings
	_ = json.Unmarshal(data, &out)
	return &out
}

var warmTimePattern = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)

func (s *MagpieSettings) validate() error {
	if s == nil {
		return nil
	}
	if !slices.Contains([]string{"", "asked"}, s.QuotaReads) || !slices.Contains([]string{"", "usd", "cny"}, s.Currency) || !slices.Contains([]string{"", "api"}, s.SearchFirst) {
		return errors.New("额度读取、货币或搜索顺序无效")
	}
	if s.UsageAlert < 0 || s.UsageAlert > 100 || math.IsNaN(s.BalanceAlert) || math.IsInf(s.BalanceAlert, 0) || s.BalanceAlert < 0 || s.ResetReminder < 0 || s.ResetReminder > 168 {
		return errors.New("额度提醒阈值无效")
	}
	if s.TrayUsageEvery < 0 || s.TrayUsageEvery > 1440 || len(s.TrayUsages) > 32 {
		return errors.New("额度显示或刷新设置无效")
	}
	for _, mode := range []string{s.CodexWarmup, s.ClaudeWarmup} {
		if !slices.Contains([]string{"", "week", "all"}, mode) {
			return errors.New("额度预热模式无效")
		}
	}
	for _, times := range [][]string{s.CodexWarmAts, s.ClaudeWarmAts} {
		if len(times) > 24 {
			return errors.New("每天最多设置 24 个预热时间")
		}
		for _, at := range times {
			if !warmTimePattern.MatchString(at) {
				return errors.New("预热时间须为 HH:MM")
			}
		}
	}
	for _, at := range []string{s.CodexWarmAt, s.ClaudeWarmAt} {
		if at != "" && !warmTimePattern.MatchString(at) {
			return errors.New("预热时间须为 HH:MM")
		}
	}
	for user, at := range s.CodexWarmAtOf {
		if len(user) > 320 || at != "off" && at != "" && !warmTimePattern.MatchString(at) {
			return errors.New("账号预热时间无效")
		}
	}
	for _, ref := range []string{s.Vision, s.ImageGen, s.Searcher} {
		if len(ref) > 400 || strings.ContainsRune(ref, '\x00') {
			return errors.New("辅助模型标识无效")
		}
	}
	seen := map[string]bool{}
	for _, api := range s.SearchAPIs {
		if !slices.Contains([]string{"tavily", "brave", "exa", "firecrawl", "searxng"}, api.Vendor) || seen[api.Vendor] || len(api.Key) > 16384 {
			return errors.New("搜索 API 无效或重复")
		}
		seen[api.Vendor] = true
		if api.URL != "" {
			u, err := url.Parse(api.URL)
			if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
				return errors.New("搜索 API 地址须为 HTTP 或 HTTPS")
			}
		}
		if api.Vendor == "searxng" && api.URL == "" {
			return errors.New("SearXNG 需要服务地址")
		}
		if api.Vendor != "searxng" && api.Key == "" && !api.HasKey {
			return errors.New("搜索 API 需要密钥")
		}
	}
	if len(s.RedactWords) > 1000 || len(s.RedactRules) > 100 {
		return errors.New("脱敏词或规则过多")
	}
	for _, word := range s.RedactWords {
		if len(word) > 1000 || strings.TrimSpace(word) == "" {
			return errors.New("脱敏词不能为空或过长")
		}
	}
	for _, rule := range s.RedactRules {
		if strings.TrimSpace(rule.Kind) == "" || len(rule.Kind) > 80 || (rule.Prefix == "") == (rule.Regex == "") {
			return errors.New("脱敏规则需要名称及一种前缀或正则表达式")
		}
		if rule.Regex != "" {
			if _, err := regexp.Compile(rule.Regex); err != nil {
				return errors.New("脱敏规则的正则表达式无效")
			}
		}
	}
	return nil
}
