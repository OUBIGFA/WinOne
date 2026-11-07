// Package automations ports Cindy's SchedulerPage and schedule form to MyGo.
// The Host's pinned Cindy scheduler remains the authority for validation,
// timing, permission defaults, dispatch, project imports and persisted records.
package automations

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"winone/desktopmygo/chat/pluginchecks"
	"winone/desktopmygo/i18n"
)

// Definition is the editable local Pi schedule contract (Cindy scheduler/types.ts).
// No permission selector is added: new tasks use the Host default and updates
// send only changed fields, so legacy permissions and future fields survive.
type Definition struct {
	Name              string        `json:"name"`
	Prompt            string        `json:"prompt"`
	Kind              string        `json:"kind"`
	CronExpr          string        `json:"cronExpr"`
	Timezone          string        `json:"timezone"`
	Recurring         bool          `json:"recurring"`
	Manual            bool          `json:"manual"`
	IntervalMs        *int64        `json:"intervalMs,omitempty"`
	OnceAt            *int64        `json:"onceAt,omitempty"`
	AgentKind         string        `json:"agentKind"`
	ModelAgentKind    string        `json:"modelAgentKind,omitempty"`
	Model             string        `json:"model,omitempty"`
	ProviderID        string        `json:"providerId,omitempty"`
	Effort            string        `json:"effort,omitempty"`
	FastMode          bool          `json:"fastMode"`
	WorkspaceKind     string        `json:"workspaceKind"`
	WorkingDir        string        `json:"workingDir,omitempty"`
	UseWorktree       bool          `json:"useWorktree"`
	TargetSessionID   string        `json:"targetSessionId,omitempty"`
	PersistentSession bool          `json:"persistentSession"`
	SilentWhenIdle    bool          `json:"silentWhenIdle"`
	ExecutionMode     string        `json:"executionMode"`
	ScriptConfig      *ScriptConfig `json:"scriptConfig,omitempty"`
	PreRunHook        *Hook         `json:"preRunHook,omitempty"`
	PreRunPlugin      *PluginCheck  `json:"preRunPlugin,omitempty"`
	Notify            Notifications `json:"notify"`
	ExpireAt          *int64        `json:"expireAt,omitempty"`
}
type ScriptConfig struct {
	Command      string   `json:"command"`
	TimeoutMs    *int64   `json:"timeoutMs,omitempty"`
	Capabilities []string `json:"capabilities"`
}
type Hook struct {
	Command   string `json:"command"`
	TimeoutMs *int64 `json:"timeoutMs,omitempty"`
}
type PluginCheck = pluginchecks.Check
type Notifications struct {
	Desktop    bool `json:"desktop"`
	Feishu     bool `json:"feishu"`
	WecomGroup bool `json:"wecomGroup,omitempty"`
}
type Schedule struct {
	Definition
	ID                 string `json:"id"`
	Source             string `json:"source,omitempty"`
	ProjectConfigID    string `json:"projectConfigId,omitempty"`
	Status             string `json:"status"`
	CreatedAt          int64  `json:"createdAt"`
	UpdatedAt          int64  `json:"updatedAt"`
	LastFiredAt        int64  `json:"lastFiredAt,omitempty"`
	NextFireAt         int64  `json:"nextFireAt,omitempty"`
	UnreadCount        int    `json:"unreadRunCount,omitempty"`
	UnreadFailureCount int    `json:"unreadFailureCount,omitempty"`
}
type Run struct {
	ID                  string          `json:"id"`
	ScheduleID          string          `json:"scheduleId"`
	SessionID           string          `json:"sessionId,omitempty"`
	FiredAt             int64           `json:"firedAt"`
	FinishedAt          int64           `json:"finishedAt,omitempty"`
	Status              string          `json:"status"`
	ErrorMsg            string          `json:"errorMsg,omitempty"`
	ResultText          string          `json:"resultText,omitempty"`
	ReadAt              int64           `json:"readAt,omitempty"`
	TotalTokens         int64           `json:"totalTokens,omitempty"`
	CostUSD             *float64        `json:"costUsd,omitempty"`
	CostMoney           *Money          `json:"costMoney,omitempty"`
	EstimatedValueMoney *Money          `json:"estimatedValueMoney,omitempty"`
	PreRunHookResult    json.RawMessage `json:"preRunHookResult,omitempty"`
	NotificationError   string          `json:"notificationError,omitempty"`
}
type Money struct {
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Approximate bool    `json:"approximate"`
}
type Runtime struct {
	Timezone           string   `json:"timezone"`
	ScriptCapabilities []string `json:"scriptCapabilities"`
	SlotsInUse         int      `json:"slotsInUse"`
	MaxConcurrentRuns  int      `json:"maxConcurrentRuns"`
	InFlightRuns       []struct {
		ScheduleID string `json:"scheduleId"`
		RunID      string `json:"runId"`
		Phase      string `json:"phase"`
		StartedAt  int64  `json:"startedAt"`
	} `json:"inFlightRuns"`
	WaitingSchedules []struct {
		ScheduleID   string `json:"scheduleId"`
		WaitingSince int64  `json:"waitingSince"`
	} `json:"waitingSchedules"`
}
type Template struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Category    string              `json:"category"`
	Source      string              `json:"source"`
	Prompt      string              `json:"prompt"`
	Parameters  []TemplateParameter `json:"parameters"`
}
type TemplateParameter struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Default     string   `json:"default"`
	Options     []string `json:"options"`
	Placeholder string   `json:"placeholder"`
}
type PendingImport struct {
	WorkingDir   string            `json:"workingDir"`
	ConfigHash   string            `json:"configHash"`
	Schedules    []json.RawMessage `json:"schedules"`
	RemovedNames []string          `json:"removedNames"`
}

// Draft remembers both the original definition and all editing buffers. Only
// its delta is sent for edits: saving a name cannot reset a stale onceAt date,
// an unusual interval or the old schedule's permission mode.
type Draft struct {
	cron            *cronForm
	ID              string
	Value           Definition
	original        Definition
	timing          string
	interval        string
	intervalUnit    string
	scriptTimeout   string
	hookTimeout     string
	preview         string
	err             string
	hookResult      string
	hookDescription string
	originalBuffers string
	previewRevision uint64
}

func newDraft(source *Schedule, timezone ...string) *Draft {
	value := Definition{Kind: "cron", CronExpr: "0 9 * * *", Timezone: time.Now().Location().String(), Recurring: true, AgentKind: "pi", WorkspaceKind: "dialogue", ExecutionMode: "agent", Notify: Notifications{Desktop: true}}
	// Windows' Local name is not an IANA zone. UTC is a valid neutral default;
	// existing stored timezones are always preserved, including unknown ones.
	if value.Timezone == "Local" || value.Timezone == "" {
		value.Timezone = "UTC"
	}
	if len(timezone) > 0 && timezone[0] != "" {
		value.Timezone = timezone[0]
	}
	if source != nil {
		value = source.Definition
	}
	raw, _ := json.Marshal(value)
	var detached Definition
	_ = json.Unmarshal(raw, &detached)
	value = detached
	d := &Draft{Value: value, intervalUnit: "minutes"}
	raw, _ = json.Marshal(value)
	_ = json.Unmarshal(raw, &d.original)
	if source != nil {
		d.ID = source.ID
	}
	d.timing = "cron"
	if value.Manual {
		d.timing = "manual"
	} else if value.IntervalMs != nil {
		d.timing = "interval"
		d.interval = fmt.Sprintf("%g", float64(*value.IntervalMs)/60000)
	}
	if value.OnceAt != nil {
		d.timing = "date"
	}
	if value.ScriptConfig != nil && value.ScriptConfig.TimeoutMs != nil {
		d.scriptTimeout = fmt.Sprintf("%g", float64(*value.ScriptConfig.TimeoutMs)/1000)
	}
	if value.PreRunHook != nil && value.PreRunHook.TimeoutMs != nil {
		d.hookTimeout = fmt.Sprintf("%g", float64(*value.PreRunHook.TimeoutMs)/1000)
	}
	d.originalBuffers = d.buffers()
	return d
}
func (d *Draft) buffers() string {
	return strings.Join([]string{d.interval, d.intervalUnit, d.scriptTimeout, d.hookTimeout}, "\x00")
}
func (d *Draft) changed() bool {
	return !reflect.DeepEqual(d.Value, d.original) || d.buffers() != d.originalBuffers
}
func (d *Draft) patch() map[string]any {
	current, original := objectOf(d.Value), objectOf(d.original)
	patch := map[string]any{}
	for key, value := range current {
		if !reflect.DeepEqual(value, original[key]) {
			patch[key] = value
		}
	}
	for key := range original {
		if _, ok := current[key]; !ok {
			patch[key] = nil
		}
	}
	return patch
}
func objectOf(value any) map[string]any {
	raw, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	return result
}
func (d *Draft) validate() error {
	v := d.Value
	if d.timing == "interval" {
		n, err := strconv.ParseFloat(d.interval, 64)
		unit := map[string]float64{"seconds": 1000, "minutes": 60000, "hours": 3600000, "days": 86400000}[d.intervalUnit]
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n*unit < 1000 || n*unit > 9007199254740991 {
			return fmt.Errorf("%s", tr("invalidInterval"))
		}
	}
	for _, text := range []string{d.scriptTimeout, d.hookTimeout} {
		if strings.TrimSpace(text) != "" {
			n, err := strconv.ParseFloat(text, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 || n > 9007199254740 {
				return fmt.Errorf("%s", tr("invalidTimeout"))
			}
		}
	}
	if strings.TrimSpace(v.Name) == "" {
		return fmt.Errorf("%s", tr("nameRequired"))
	}
	if v.ExecutionMode != "script" && strings.TrimSpace(v.Prompt) == "" {
		return fmt.Errorf("%s", tr("promptRequired"))
	}
	if v.ExecutionMode == "script" && (v.ScriptConfig == nil || strings.TrimSpace(v.ScriptConfig.Command) == "") {
		return fmt.Errorf("%s", tr("commandRequired"))
	}
	if v.WorkspaceKind == "project" && v.WorkingDir == "" && v.TargetSessionID == "" {
		return fmt.Errorf("%s", tr("directoryRequired"))
	}
	if v.TargetSessionID == "__pending__" {
		return fmt.Errorf("%s", tr("sessionRequired"))
	}
	if v.PreRunHook != nil && strings.TrimSpace(v.PreRunHook.Command) == "" {
		return fmt.Errorf("%s", tr("commandRequired"))
	}
	if v.PreRunPlugin != nil && v.PreRunPlugin.PluginID == "" {
		return fmt.Errorf("%s", tr("pluginRequired"))
	}
	return nil
}

// Cindy sorts active and expired together, pauses last, then latest run and
// update first. Directory and text filters do not mutate the underlying order.
func visible(items []Schedule, filter, search, directory string) []Schedule {
	result := []Schedule{}
	for _, s := range items {
		if filter == "active" && s.Status == "paused" || filter == "paused" && s.Status != "paused" {
			continue
		}
		if directory != "" && !strings.EqualFold(strings.TrimRight(s.WorkingDir, "/\\"), strings.TrimRight(directory, "/\\")) {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(s.Name+" "+s.Prompt+" "+s.WorkingDir), strings.ToLower(search)) {
			continue
		}
		result = append(result, s)
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if (a.Status == "paused") != (b.Status == "paused") {
			return a.Status != "paused"
		}
		if a.LastFiredAt != b.LastFiredAt {
			return a.LastFiredAt > b.LastFiredAt
		}
		return a.UpdatedAt > b.UpdatedAt
	})
	return result
}

// A hand-bound owner's conversation is not generated by an automation. Never
// archive or delete it when the user removes the automation's generated runs.
func generatedSessionIDs(schedule Schedule, runs []Run) []string {
	seen := map[string]bool{}
	ids := []string{}
	for _, r := range runs {
		if r.SessionID != "" && r.SessionID != schedule.TargetSessionID && !seen[r.SessionID] {
			seen[r.SessionID] = true
			ids = append(ids, r.SessionID)
		}
	}
	return ids
}
func locale() string                    { return i18n.Locale() }
func tr(key string, args ...any) string { return i18n.T("nativeAutomation."+key, args...) }
func dateText(value int64) string {
	if value == 0 {
		return "—"
	}
	return time.UnixMilli(value).Local().Format("2006-01-02 15:04:05")
}
