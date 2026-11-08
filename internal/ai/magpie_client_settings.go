package ai

import (
	"errors"
	"net/url"
	"slices"
	"strings"
)

// MagpieClientSettings is shared by the native draft and Host. These options
// affect requests through WinOne; they never rewrite an external app's files.
type MagpieClientSettings struct {
	UsageOrder               []string            `json:"usageOrder,omitempty"`
	UsageAgentsHidden        []string            `json:"usageAgentsHidden,omitempty"`
	GitHubToken              *string             `json:"githubToken,omitempty"`
	HasGitHubToken           bool                `json:"hasGitHubToken,omitempty"`
	OTel                     Observability       `json:"otel,omitempty"`
	CodexAsyncCompact        bool                `json:"codexAsyncCompact,omitempty"`
	CodexAsyncCompactPercent int                 `json:"codexAsyncCompactPercent,omitempty"`
	VisionEffort             string              `json:"visionEffort,omitempty"`
	SearcherEffort           string              `json:"searcherEffort,omitempty"`
	BareModelIDs             bool                `json:"bareModelIDs,omitempty"`
	CodexDescriptions        string              `json:"codexDescriptions,omitempty"`
	CodexPredictions         string              `json:"codexPredictions,omitempty"`
	DesktopSubagentModel     string              `json:"desktopSubagentModel,omitempty"`
	DesktopSubagentEffort    string              `json:"desktopSubagentEffort,omitempty"`
	DesktopLongest           bool                `json:"desktopLongest,omitempty"`
	CodexAutoResetOnly       map[string]string   `json:"codexAutoResetOnly,omitempty"`
	CheckinOff               []string            `json:"checkinOff,omitempty"`
	AgentEfforts             map[string]string   `json:"agentEfforts,omitempty"`
	FastPicks                map[string][]string `json:"fastPicks,omitempty"`
	UltraPicks               map[string][]string `json:"ultraPicks,omitempty"`
	Visible                  map[string][]string `json:"visible,omitempty"`
	HiddenModels             map[string][]string `json:"hiddenModels,omitempty"`
	DisabledModels           []string            `json:"disabledModels,omitempty"`
	PickedModels             map[string][]string `json:"pickedModels,omitempty"`
	OwnModelsHidden          []string            `json:"ownModelsHidden,omitempty"`
	OrderedModels            map[string][]string `json:"orderedModels,omitempty"`
	ClientAgents             map[string]string   `json:"clientAgents,omitempty"`
}

// Observability is opt-in. The upstream observer owns OTLP export and flush;
// transport workers never export the outer request a second time.
type Observability struct {
	Enabled     bool              `json:"enabled,omitempty"`
	Endpoint    string            `json:"endpoint,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Metrics     bool              `json:"metrics,omitempty"`
	Sessions    bool              `json:"sessions,omitempty"`
	Bodies      bool              `json:"bodies,omitempty"`
	BodiesWhole bool              `json:"bodiesWhole,omitempty"`
}

// These client choices require upstream's client-aware catalog and request
// handlers even when user-created routing groups are not enabled.
func (s MagpieClientSettings) UsesClientRules() bool {
	return s.BareModelIDs || s.CodexDescriptions != "" || s.CodexPredictions != "" || s.DesktopSubagentModel != "" || s.DesktopSubagentEffort != "" || s.DesktopLongest || len(s.AgentEfforts)+len(s.FastPicks)+len(s.UltraPicks)+len(s.Visible)+len(s.HiddenModels)+len(s.DisabledModels)+len(s.PickedModels)+len(s.OwnModelsHidden)+len(s.OrderedModels) > 0
}

// Use upstream's disabled-model matcher (including ? and slash-spanning *),
// also at the Host boundary where private account IDs differ from vendor IDs.
func (s MagpieClientSettings) ModelDisabled(pid, model, alias string) bool {
	names := namesOf(pid, model)
	if alias != "" {
		names = append(names, namesOf(pid, alias)...)
	}
	_, off := matchDisabled(s.DisabledModels, names...)
	return off
}

func (s MagpieClientSettings) validate() error {
	if s.CodexAsyncCompactPercent < 0 || s.CodexAsyncCompactPercent > 100 {
		return errors.New("后台压缩阈值需在 0 到 100 之间")
	}
	if s.OTel.Enabled && s.OTel.Endpoint == "" {
		return errors.New("请填写观测数据接收地址")
	}
	if s.OTel.Endpoint != "" {
		u, err := url.Parse(s.OTel.Endpoint)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("观测数据接收地址无效")
		}
	}
	if len(s.OTel.Headers) > 32 {
		return errors.New("观测请求头过多")
	}
	if err := validateProviderHeaders(s.OTel.Headers); err != nil {
		return err
	}
	if len(s.ClientAgents) > 512 {
		return errors.New("客户端归类过多")
	}
	for from, to := range s.ClientAgents {
		if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" || len(from) > 256 || len(to) > 256 || strings.ContainsAny(from+to, "\x00\r\n") {
			return errors.New("客户端归类无效")
		}
	}
	for _, ref := range []string{s.CodexDescriptions, s.CodexPredictions, s.DesktopSubagentModel} {
		if len(ref) > 1024 || strings.ContainsRune(ref, '\x00') || ref != "" && ref != "off" && !strings.Contains(ref, "/") {
			return errors.New("客户端模型需要供应商/模型标识")
		}
	}
	levels := []string{"", "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}
	for _, effort := range []string{s.VisionEffort, s.SearcherEffort, s.DesktopSubagentEffort} {
		if !slices.Contains(levels, effort) {
			return errors.New("辅助模型思考等级无效")
		}
	}
	if s.DesktopSubagentModel == "off" || s.DesktopSubagentEffort == "ultra" {
		return errors.New("Claude Desktop 子代理设置无效")
	}
	for _, effort := range s.AgentEfforts {
		if effort == "ultra" || !slices.Contains(levels, effort) {
			return errors.New("客户端思考等级无效")
		}
	}
	for _, when := range s.CodexAutoResetOnly {
		if when != "" && when != "week" && when != "expiring" {
			return errors.New("自动重置时机无效")
		}
	}
	if len(s.DisabledModels) > 4096 {
		return errors.New("禁用模型规则过多")
	}
	for _, rule := range s.DisabledModels {
		if len(rule) > 1024 || strings.ContainsRune(rule, '\x00') {
			return errors.New("禁用模型规则无效")
		}
	}
	for _, mapping := range []map[string][]string{s.Visible, s.HiddenModels, s.PickedModels, s.OrderedModels, s.FastPicks, s.UltraPicks} {
		if len(mapping) > 512 {
			return errors.New("客户端设置过多")
		}
		for client, refs := range mapping {
			if client == "" || len(client) > 256 || len(refs) > MaxProviderModels {
				return errors.New("客户端模型列表无效")
			}
			for _, ref := range refs {
				if ref == "" || len(ref) > 1024 || strings.ContainsRune(ref, '\x00') {
					return errors.New("客户端模型标识无效")
				}
			}
		}
	}
	return nil
}
