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
	MagpieClientSettings
	GroupSink       bool `json:"groupSink,omitempty"`
	GroupFirstToken int  `json:"groupFirstToken,omitempty"`
	GroupPatience   int  `json:"groupPatience,omitempty"`
	// Current Magpie options, persisted with the existing WinOne transaction.
	ReplyModel         string                  `json:"replyModel,omitempty"`
	BusyLoad           int                     `json:"busyLoad,omitempty"`
	SearcherThen       []string                `json:"searcherThen,omitempty"`
	SearchAll          bool                    `json:"searchAll,omitempty"`
	SearchCount        int                     `json:"searchCount,omitempty"`
	RetryAfterCap      *int                    `json:"retryAfterCap,omitempty"`
	RefusalRetries     *int                    `json:"refusalRetries,omitempty"`
	CodexSubagentModel string                  `json:"codexSubagentModel,omitempty"`
	ClaudePrintMode    bool                    `json:"claudePrintMode,omitempty"`
	SignInLinkOnly     bool                    `json:"signInLinkOnly,omitempty"`
	RedactWordsWhole   bool                    `json:"redactWordsWhole,omitempty"`
	RedactRouteTo      string                  `json:"redactRouteTo,omitempty"`
	MaskAlert          bool                    `json:"maskAlert,omitempty"`
	NPMRegistry        string                  `json:"npmRegistry,omitempty"`
	ChinaMirror        bool                    `json:"chinaMirror,omitempty"`
	RouterNote         string                  `json:"routerNote,omitempty"`
	RouterAllTools     bool                    `json:"routerAllTools,omitempty"`
	AgentCompacts      map[string]AgentCompact `json:"agentCompacts,omitempty"`
	CodexResetLead     int                     `json:"codexResetLead,omitempty"`
	CodexCreditsLast   []string                `json:"codexCreditsLast,omitempty"`
	CodexCreditsFirst  []string                `json:"codexCreditsFirst,omitempty"`
	TrayWindows        map[string][]string     `json:"trayWindows,omitempty"`
	TrayNoNumber       bool                    `json:"trayNoNumber,omitempty"`
	UsageCardsHidden   []string                `json:"usageCardsHidden,omitempty"`
	UsageCleared       []UsageClear            `json:"usageCleared,omitempty"`
	// These are populated only from the private local-model profile.
	RedactModelName       string             `json:"redactModelName,omitempty"`
	RedactModelScore      map[string]float64 `json:"redactModelScore,omitempty"`
	RequestArchiveLocal   bool               `json:"requestArchiveLocal,omitempty"`
	RequestArchiveLocalMB int                `json:"requestArchiveLocalMB,omitempty"`
	RequestArchiveMaxMB   int                `json:"requestArchiveMaxMB,omitempty"`
	Proxy                 string             `json:"proxy,omitempty"`
	ProxyCustom           string             `json:"proxyCustom,omitempty"`
	ClassifyTimeout       int                `json:"classifyTimeout,omitempty"` // Seconds; 0 follows Magpie's 8-second default.
	ClaudeLeanPrompt      bool               `json:"claudeLeanPrompt,omitempty"`
	ClaudeRunsMost        int                `json:"claudeRunsMost,omitempty"` // 0 retains upstream automatic sizing.
	// Empty preserves the upstream public-address-only download policy.
	FetchPrivate    []string                       `json:"fetchPrivate,omitempty"`
	BuiltinPreset   string                         `json:"builtinPreset,omitempty"`
	BuiltinPlugins  map[string]BuiltinPluginChoice `json:"builtinPlugins,omitempty"`
	BundledPlugins  map[string]BundledPluginChoice `json:"bundledPlugins,omitempty"`
	PluginOptions   map[string]map[string]bool     `json:"pluginOptions,omitempty"`
	CodexAgentsV1   bool                           `json:"codexAgentsV1,omitempty"`
	FullContext     bool                           `json:"fullContext,omitempty"`
	CompactAt       int                            `json:"compactAt,omitempty"`
	CodexTitles     string                         `json:"codexTitles,omitempty"`
	CodexAutoReview string                         `json:"codexAutoReview,omitempty"`
	// Filled only in the private runtime snapshot for historical usage pricing.
	ModelPrices map[string]ModelPrice `json:"modelPrices,omitempty"`
	ModelSameAs map[string]string     `json:"modelSameAs,omitempty"`
	// Display-only grouping. It never changes routing IDs, requests or prices.
	UsageAliases     map[string]string `json:"usageAliases,omitempty"`
	QuotaLeft        bool              `json:"quotaLeft,omitempty"`
	QuotaReads       string            `json:"quotaReads,omitempty"`
	Currency         string            `json:"currency,omitempty"`
	ChineseUnits     bool              `json:"chineseUnits,omitempty"`
	UsageAlert       int               `json:"usageAlert,omitempty"`
	BalanceAlert     float64           `json:"balanceAlert,omitempty"`
	ResetReminder    int               `json:"resetReminder,omitempty"`
	TrayUsages       []string          `json:"trayUsages,omitempty"`
	TrayUsageEvery   int               `json:"trayUsageEvery,omitempty"`
	TrayNoLogos      bool              `json:"trayNoLogos,omitempty"`
	TrayNoBird       bool              `json:"trayNoBird,omitempty"`
	CodexWarmup      string            `json:"codexWarmup,omitempty"`
	ClaudeWarmup     string            `json:"claudeWarmup,omitempty"`
	CodexWarmAt      string            `json:"codexWarmAt,omitempty"`
	ClaudeWarmAt     string            `json:"claudeWarmAt,omitempty"`
	CodexWarmAts     []string          `json:"codexWarmAts,omitempty"`
	ClaudeWarmAts    []string          `json:"claudeWarmAts,omitempty"`
	CodexWarmAtOf    map[string]string `json:"codexWarmAtOf,omitempty"`
	CodexAutoReset   []string          `json:"codexAutoReset,omitempty"`
	CodexNoCredits   []string          `json:"codexNoCredits,omitempty"`
	WorkBuddyCheckin bool              `json:"workbuddyCheckin,omitempty"`
	TraeCheckin      bool              `json:"traeCheckin,omitempty"`
	MiniMaxCheckin   bool              `json:"minimaxCheckin,omitempty"`
	QoderCheckin     bool              `json:"qoderCheckin,omitempty"`
	PluginCheckins   map[string]bool   `json:"pluginCheckins,omitempty"`
	Vision           string            `json:"vision,omitempty"`
	ImageGen         string            `json:"imageGen,omitempty"`
	// Upstream's explicit model kind map; absent follows its own catalog.
	ModelKinds    map[string]string `json:"modelKinds,omitempty"`
	ImageTimeout  int               `json:"imageTimeout,omitempty"` // Minutes; 0 uses Magpie's 5-minute default.
	Searcher      string            `json:"searcher,omitempty"`
	SearchFirst   string            `json:"searchFirst,omitempty"`
	SearchAPIs    []SearchAPI       `json:"searchAPIs,omitempty"`
	MemberModel   bool              `json:"memberModel,omitempty"`
	NoLoopGuard   bool              `json:"noLoopGuard,omitempty"`
	PlainNames    bool              `json:"plainNames,omitempty"`
	PlainOwnNames bool              `json:"plainOwnNames,omitempty"`
	Redact        bool              `json:"redact,omitempty"`
	// Preserve upstream key-shape masking; false keeps placeholder defaults.
	RedactShape        bool              `json:"redactShape,omitempty"`
	RedactPersonal     bool              `json:"redactPersonal,omitempty"`
	RedactKinds        map[string]bool   `json:"redactKinds,omitempty"`
	RedactWords        []string          `json:"redactWords,omitempty"`
	RedactWordsAnyCase bool              `json:"redactWordsAnyCase,omitempty"`
	RedactActions      map[string]string `json:"redactActions,omitempty"`
	RedactAllow        []string          `json:"redactAllow,omitempty"`
	RedactRules        []RedactRule      `json:"redactRules,omitempty"`
	HideAccounts       bool              `json:"hideAccounts,omitempty"`
	NoStats            bool              `json:"noStats"`
	NoUsageStats       bool              `json:"noUsageStats"`
	NoAccountDiscovery bool              `json:"noAccountDiscovery,omitempty"`
}

// AgentCompact keeps the upstream units and fallback semantics; absent entries
// inherit the global window. These settings never authorize external writes.
type AgentCompact struct {
	At   int  `json:"at,omitempty"`
	Full bool `json:"full,omitempty"`
}

type UsageClear struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	At       int64  `json:"at"`
}

// Plugin choices are one saved WinOne draft, not account-local YAML files.
type BuiltinPluginChoice struct {
	Enabled *bool          `json:"enabled,omitempty"`
	Config  map[string]any `json:"config,omitempty"`
}
type BundledPluginChoice struct {
	Enabled bool           `json:"enabled"`
	Removed bool           `json:"removed,omitempty"`
	Options map[string]any `json:"options,omitempty"`
}

// Key is accepted only on save or in a private runtime snapshot. Public reads
// carry HasKey, and explicit reveal is a separate, user-triggered operation.
type SearchAPI struct {
	Vendor  string       `json:"vendor"`
	URL     string       `json:"url,omitempty"`
	Key     string       `json:"key,omitempty"`
	HasKey  bool         `json:"hasKey,omitempty"`
	Pick    string       `json:"pick,omitempty"` // Empty rotates keys; order tries them in declared order.
	Off     bool         `json:"off,omitempty"`  // Disabled APIs keep their credentials and position.
	Mix     bool         `json:"mix,omitempty"`  // Ask marked APIs together and merge their results.
	KeyList *[]SearchKey `json:"keyList,omitempty"`
}

// Public slots identify separately encrypted keys; runtime snapshots alone
// contain Key. A nil list preserves the legacy comma-separated credential.
type SearchKey struct {
	ID     string `json:"id,omitempty"`
	Key    string `json:"key,omitempty"`
	Off    bool   `json:"off,omitempty"`
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
	if err := s.MagpieClientSettings.validate(); err != nil {
		return err
	}
	if s.GroupFirstToken < 0 || s.GroupPatience < -1 || s.GroupPatience > 240 {
		return errors.New("路由默认等待时间无效")
	}
	if s.BusyLoad < 0 || s.BusyLoad > 9999 || s.CodexResetLead < 0 || s.CodexResetLead > 1440 || (s.CodexResetLead > 0 && s.CodexResetLead < 3) {
		return errors.New("繁忙模型阈值或提前重置时间无效")
	}
	if len(s.AgentCompacts) > 3 {
		return errors.New("客户端上下文设置过多")
	}
	for agent, compact := range s.AgentCompacts {
		if !slices.Contains([]string{"codex", "claude", "grok"}, agent) || compact.At < 0 || compact.At > 100000000 {
			return errors.New("客户端上下文设置无效")
		}
	}
	if s.SearchCount < 0 || s.SearchCount > 20 {
		return errors.New("搜索结果数须为 1–20，0 使用默认 6")
	}
	if s.RetryAfterCap != nil && (*s.RetryAfterCap < 0 || *s.RetryAfterCap > 604800) {
		return errors.New("最长重试等待须为 0–604800 秒")
	}
	if ref := s.CodexSubagentModel; len(ref) > 1024 || strings.ContainsRune(ref, '\x00') || ref != "" && ref != "lead" && !strings.Contains(ref, "/") {
		return errors.New("Codex 子代理模型须为 lead 或供应商/模型标识")
	}
	if !slices.Contains([]string{"", "asked", "member", "vendor"}, s.ReplyModel) {
		return errors.New("回复模型名称模式无效")
	}
	if len(s.SearcherThen) > 64 || len(s.UsageCleared) > 4096 || len(s.UsageCardsHidden) > 4096 {
		return errors.New("搜索链或用量显示选项过多")
	}
	for _, entry := range s.UsageCleared {
		if entry.At < 0 || len(entry.Provider) > 1024 || len(entry.Model) > 1024 {
			return errors.New("用量清零标识无效")
		}
	}
	if s.NPMRegistry != "" {
		u, err := url.Parse(s.NPMRegistry)
		if err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("npm 注册表需要无凭据的 HTTP 或 HTTPS 地址")
		}
	}
	for _, value := range []string{s.RedactRouteTo, s.RouterNote} {
		if len(value) > 4096 || strings.ContainsRune(value, '\x00') {
			return errors.New("隐私模型路由或 MCP 说明无效")
		}
	}
	for _, value := range []string{s.Proxy, s.ProxyCustom} {
		if len(value) > 4096 {
			return errors.New("代理地址过长")
		}
		if err := validateMagpieProxy(value); err != nil {
			return errors.New("代理地址无效，请填写 HTTP、HTTPS 或 SOCKS5 地址")
		}
	}
	if len(s.UsageAliases) > 4096 {
		return errors.New("用量归并别名过多")
	}
	for from, to := range s.UsageAliases {
		if len(from) > 1024 || len(to) > 1024 || strings.ContainsRune(from, '\x00') || strings.ContainsRune(to, '\x00') {
			return errors.New("用量归并标识格式无效")
		}
	}
	if s.ClassifyTimeout < 0 || s.ClassifyTimeout > 60 {
		return errors.New("路由判断超时须为 1–60 秒，0 使用默认 8 秒")
	}
	if s.ClaudeRunsMost < 0 || s.ClaudeRunsMost > 256 {
		return errors.New("Claude 并发进程数须为 0–256，0 使用自动配置")
	}
	if s.ImageTimeout < 0 || s.ImageTimeout > 60 {
		return errors.New("图片生成超时须为 1–60 分钟，0 使用默认 5 分钟")
	}
	if s.RequestArchiveMaxMB < 0 || s.RequestArchiveMaxMB > 1024 || s.RequestArchiveLocalMB < 0 || s.RequestArchiveLocalMB > 1<<20 {
		return errors.New("请求归档容量无效")
	}
	if len(s.FetchPrivate) > 128 {
		return errors.New("最多允许 128 个内网下载地址")
	}
	for _, entry := range s.FetchPrivate {
		if entry == "" || len(entry) > 512 || strings.Contains(entry, "://") || strings.ContainsAny(entry, " \t\r\n\x00?#@") {
			return errors.New("内网下载地址须为主机名、IP、主机与端口或 CIDR 网段")
		}
	}
	if err := s.validatePlugins(); err != nil {
		return err
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
	if len(s.ModelKinds) > MaxProviderModels {
		return errors.New("媒体模型类型条目过多")
	}
	for id, kind := range s.ModelKinds {
		if !strings.Contains(id, "/") || len(id) > 512 || strings.ContainsRune(id, '\x00') || kind != "image" && kind != "video" {
			return errors.New("媒体模型类型须使用供应商/模型 ID 和 image 或 video")
		}
	}
	seen := map[string]bool{}
	for _, api := range s.SearchAPIs {
		if !slices.Contains([]string{"tavily", "brave", "exa", "firecrawl", "searxng", "deepseek", "you", "volcengine", "wsa"}, api.Vendor) || seen[api.Vendor] || len(api.Key) > 16384 {
			return errors.New("搜索 API 无效或重复")
		}
		seen[api.Vendor] = true
		if api.KeyList != nil {
			ids := map[string]bool{}
			if len(*api.KeyList) > 256 {
				return errors.New("搜索 API 密钥数量过多")
			}
			for _, key := range *api.KeyList {
				if key.ID == "" || len(key.ID) > 128 || strings.ContainsAny(key.ID, "/\\\x00") || ids[key.ID] || len(key.Key) > 16384 || key.Key == "" && !key.HasKey {
					return errors.New("搜索 API 密钥条目无效或重复")
				}
				ids[key.ID] = true
			}
		}
		if api.Pick != "" && api.Pick != "order" {
			return errors.New("搜索 API 密钥选择策略无效")
		}
		if api.URL != "" {
			u, err := url.Parse(api.URL)
			if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
				return errors.New("搜索 API 地址须为 HTTP 或 HTTPS")
			}
		}
		if api.Vendor == "searxng" && api.URL == "" {
			return errors.New("SearXNG 需要服务地址")
		}
		if api.Vendor != "searxng" && api.Vendor != "you" && api.Key == "" && !api.HasKey && api.KeyList == nil && !api.Off {
			return errors.New("搜索 API 需要密钥")
		}
	}
	if len(s.RedactActions) > 256 || len(s.RedactAllow) > 1000 {
		return errors.New("脱敏动作或允许值过多")
	}
	for kind, action := range s.RedactActions {
		if !regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`).MatchString(kind) || !slices.Contains([]string{"mask", "warn", "block"}, action) {
			return errors.New("脱敏类型或动作无效")
		}
	}
	for _, value := range s.RedactAllow {
		if strings.TrimSpace(value) == "" || len(value) > 1000 {
			return errors.New("允许值不能为空或过长")
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
