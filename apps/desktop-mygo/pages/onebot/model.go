package onebot

import (
	"regexp"
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

// These are the existing Host projections, not a second profile store. Unknown
// capability fields survive native edits, including imported policy metadata.
type ModelRoute struct {
	Harness    string          `json:"harness"`
	ProviderID *string         `json:"providerId"`
	Model      string          `json:"model"`
	Effort     string          `json:"effort"`
	FastMode   json.RawMessage `json:"fastMode"`
}
type Capabilities struct {
	Model                string            `json:"model"`
	ProviderID           *string           `json:"providerId,omitempty"`
	Effort               string            `json:"effort"`
	FastMode             json.RawMessage   `json:"fastMode"`
	Harness              string            `json:"harness"`
	ModelChain           []ModelRoute      `json:"modelChain"`
	ModelChainOverride   json.RawMessage   `json:"modelChainOverride,omitempty"`
	ModelOverride        json.RawMessage   `json:"modelOverride,omitempty"`
	TaskModelOverride    *ModelRoute       `json:"taskModelOverride,omitempty"`
	SkillMode            string            `json:"skillMode"`
	SkillsExcluded       []string          `json:"skillsExcluded,omitempty"`
	ToolsetMode          string            `json:"toolsetMode"`
	Toolsets             []string          `json:"toolsets"`
	MCPMode              string            `json:"mcpMode"`
	MCPServers           []string          `json:"mcpServers"`
	MCPServerNames       map[string]string `json:"mcpServerNames,omitempty"`
	Memory               bool              `json:"memory"`
	Permissions          string            `json:"permissions"`
	InputHandoff         *bool             `json:"inputHandoff,omitempty"`
	AllowSchedulerFollow bool              `json:"allowSchedulerFollow,omitempty"`
	SystemSkillsOff      []string          `json:"systemSkillsOff,omitempty"`
}
type BotSession struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Kind      string `json:"kind"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	GroupID   string `json:"groupId,omitempty"`
	UpdatedAt int64  `json:"updatedAt"`
}
type Invitation struct {
	Stage         string `json:"stage"`
	Error         string `json:"error"`
	AvatarSkipped bool   `json:"avatarSkipped"`
}
type Profile struct {
	ID                 string                       `json:"id"`
	Name               string                       `json:"name"`
	Description        string                       `json:"description"`
	Gender             string                       `json:"gender,omitempty"`
	Avatar             string                       `json:"avatar"`
	AvatarColor        string                       `json:"avatarColor"`
	Status             string                       `json:"status"`
	Enabled            bool                         `json:"enabled"`
	HiddenAt           *int64                       `json:"hiddenAt"`
	PinnedAt           *int64                       `json:"pinnedAt"`
	CurrentVersion     int                          `json:"currentVersion"`
	CanonicalSessionID string                       `json:"canonicalSessionId"`
	IdentitySource     string                       `json:"identitySource"`
	UserContextSource  string                       `json:"userContextSource"`
	HomeDir            string                       `json:"homeDir"`
	TemplateID         string                       `json:"templateId"`
	Skills             []string                     `json:"skills"`
	Capabilities       core.Preserved[Capabilities] `json:"capabilities"`
	Invitation         *Invitation                  `json:"invitation"`
	NeedsAttention     bool                         `json:"needsAttention"`
	FailureReason      string                       `json:"failureReason"`
	LastMessageText    string                       `json:"lastMessageText"`
	LastMessageAt      *int64                       `json:"lastMessageAt"`
	LastMessageRole    string                       `json:"lastMessageRole"`
	UnreadCount        int                          `json:"unreadCount"`
	CreatedAt          int64                        `json:"createdAt"`
	Sessions           []BotSession                 `json:"sessions"`
}

func bt(key string, args ...any) string { return i18n.T("onebotCindy."+key, args...) }
func nt(key string, args ...any) string { return i18n.T("nativeOneBot."+key, args...) }
func botLocale() string {
	if i18n.Locale() == i18n.English {
		return "en"
	}
	return "zh-CN"
}
func canonical(bot Profile) string {
	for _, session := range bot.Sessions {
		if session.Role == "canonical" && session.Status != "deleted" {
			return session.ID
		}
	}
	return bot.CanonicalSessionID
}
func normalizedName(name string) string {
	return strings.ToLower(strings.TrimSpace(norm.NFKC.String(name)))
}
var (
	previewFence  = regexp.MustCompile("(?s)```.*?(```|$)")
	previewLink   = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	previewMarker = regexp.MustCompile("(?m)^\\s{0,3}(?:#{1,6}\\s+|>\\s?|[-*+]\\s+)|\\*\\*|__|`|~~")
	previewSpaces = regexp.MustCompile(`\s+`)
)

// markdownPreview is a reply as one plain line (Cindy markdownPreviewText): the
// sidebar shows words, not fences or markers.
func markdownPreview(text string) string {
	text = previewFence.ReplaceAllString(text, " ")
	text = previewLink.ReplaceAllString(text, "$1")
	text = previewMarker.ReplaceAllString(text, "")
	line := []rune(strings.TrimSpace(previewSpaces.ReplaceAllString(text, " ")))
	return string(line[:min(len(line), 140)])
}

func rosterLabel(bot Profile, all []Profile) string {
	var matches []Profile
	for _, row := range all {
		if normalizedName(row.Name) == normalizedName(bot.Name) {
			matches = append(matches, row)
		}
	}
	if len(matches) < 2 {
		return bot.Name
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].CreatedAt == matches[j].CreatedAt {
			return matches[i].ID < matches[j].ID
		}
		return matches[i].CreatedAt < matches[j].CreatedAt
	})
	for index, row := range matches {
		if row.ID == bot.ID {
			return nt("duplicateName", "name", bot.Name, "number", index+1)
		}
	}
	return bot.Name
}
func copyValue[T any](value T) T {
	raw, _ := json.Marshal(value)
	var next T
	_ = json.Unmarshal(raw, &next)
	return next
}
func sameJSON(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return bytes.Equal(left, right)
}

// Only edited fields go to profiles.update. A delayed save cannot roll an
// externally changed portrait, title, permission or import setting backwards.
func profilePatch(before, after Profile) map[string]any {
	out := map[string]any{"id": after.ID}
	for _, field := range []struct {
		name      string
		old, next any
	}{
		{"name", before.Name, after.Name}, {"description", before.Description, after.Description},
		{"identitySource", before.IdentitySource, after.IdentitySource}, {"userContextSource", before.UserContextSource, after.UserContextSource},
		{"gender", before.Gender, after.Gender}, {"avatar", before.Avatar, after.Avatar}, {"avatarColor", before.AvatarColor, after.AvatarColor},
		{"skills", before.Skills, after.Skills},
	} {
		if !sameJSON(field.old, field.next) {
			out[field.name] = field.next
		}
	}
	if capabilities := objectChanges(before.Capabilities, after.Capabilities); len(capabilities) > 0 {
		out["capabilities"] = capabilities
	}
	if _, ok := out["avatar"]; ok {
		out["expectedAvatar"] = before.Avatar
	}
	return out
}

func objectChanges(before, after any) map[string]json.RawMessage {
	oldBytes, _ := json.Marshal(before)
	nextBytes, _ := json.Marshal(after)
	var old, next map[string]json.RawMessage
	_ = json.Unmarshal(oldBytes, &old)
	_ = json.Unmarshal(nextBytes, &next)
	changes := map[string]json.RawMessage{}
	for key, value := range next {
		if !bytes.Equal(old[key], value) {
			changes[key] = value
		}
	}
	for key := range old {
		if _, ok := next[key]; !ok {
			changes[key] = json.RawMessage("null")
		}
	}
	return changes
}

// Rebase host metadata and untouched fields without overwriting local edits.
// This also applies to capabilities one key at a time: changing memory does
// not rewind an independently changed model, permission or skill reference.
func rebaseProfile(current, baseline, incoming Profile) Profile {
	raw, _ := json.Marshal(incoming)
	var merged map[string]json.RawMessage
	_ = json.Unmarshal(raw, &merged)
	for key, value := range profilePatch(baseline, current) {
		if key == "id" || key == "expectedAvatar" {
			continue
		}
		if key == "capabilities" {
			var capabilities map[string]json.RawMessage
			_ = json.Unmarshal(merged[key], &capabilities)
			for name, changed := range value.(map[string]json.RawMessage) {
				capabilities[name] = changed
			}
			merged[key], _ = json.Marshal(capabilities)
		} else {
			merged[key], _ = json.Marshal(value)
		}
	}
	raw, _ = json.Marshal(merged)
	var next Profile
	_ = json.Unmarshal(raw, &next)
	return next
}
