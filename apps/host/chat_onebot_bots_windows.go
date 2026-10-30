//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"winone/internal/ai"
	"winone/internal/chat"
	"winone/internal/recycle"
	"winone/internal/settings"
)

// Cindy's teammate data paths (main/localDb/ipc/bots.ts, b7e46bc) on WinOne:
// the Worker owns profiles and links, Chat owns conversation records, and the
// Host joins them. Renderer input never names a transcript path or a model.

const oneBotAvatarSourceLimit = 20 << 20 // Cindy AVATAR_SOURCE_MAX_BYTES
const oneBotAvatarLimit = 5 << 20        // Cindy BOT_AVATAR_MAX_BYTES

var oneBotManagedMedia = regexp.MustCompile(`^cindy-media://blobs/([0-9a-f]{64})\.(png|jpe?g|webp)$`)

func oneBotImageType(data []byte) string {
	switch {
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "image/jpeg"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	}
	return ""
}

func readOneBotImage(path string, limit int64) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("头像文件无法读取")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, errors.New("头像文件无法读取")
	}
	if len(data) == 0 || int64(len(data)) > limit {
		return nil, fmt.Errorf("头像图片必须小于 %d MB", limit>>20)
	}
	mime := oneBotImageType(data)
	if mime == "" {
		return nil, errors.New("头像只支持 PNG、JPEG 或 WebP 图片")
	}
	return map[string]string{"base64": base64.StdEncoding.EncodeToString(data), "mimeType": mime}, nil
}

// avatarSource reads the image the user picked in the native file dialog for
// the crop editor (Cindy choose-avatar-source). Bytes are checked, not trusted.
func (b *chatOneBot) avatarSource(raw json.RawMessage) (any, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || !filepath.IsAbs(input.Path) {
		return nil, errors.New("请选择本机图片文件")
	}
	return readOneBotImage(input.Path, oneBotAvatarSourceLimit)
}

// mediaRead returns managed avatar bytes for display; only the content-addressed
// store under the OneBot data directory is readable through this method.
func (b *chatOneBot) mediaRead(raw json.RawMessage) (any, error) {
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	match := oneBotManagedMedia.FindStringSubmatch(input.URL)
	if match == nil {
		return nil, errors.New("不支持的媒体地址")
	}
	root := filepath.Join(b.control.directory, "onebot", "media", "blobs")
	file := filepath.Join(root, match[1]+"."+match[2])
	if !oneBotInside(root, file) {
		return nil, errors.New("不支持的媒体地址")
	}
	return readOneBotImage(file, oneBotAvatarLimit)
}

// Teammate default model chain (Cindy bot-model-chain settings). Without a
// saved chain, teammates follow Chat's default model and thinking level.
type oneBotModelChainSettings struct {
	ModelChain []oneBotModelRoute `json:"modelChain"`
}

func (b *chatOneBot) modelChainFile() string {
	return filepath.Join(b.control.directory, "onebot", "bot-model-chain.json")
}

func (b *chatOneBot) customModelChain() ([]oneBotModelRoute, error) {
	raw, err := os.ReadFile(b.modelChainFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var saved oneBotModelChainSettings
	if err = json.Unmarshal(raw, &saved); err != nil {
		return nil, fmt.Errorf("伙伴默认模型设置无法读取：%w", err)
	}
	return saved.ModelChain, validateOneBotModelChain(saved.ModelChain)
}

func validateOneBotModelChain(chain []oneBotModelRoute) error {
	if len(chain) == 0 || len(chain) > 5 {
		return errors.New("伙伴模型链需要 1 至 5 条路线")
	}
	for _, route := range chain {
		var fast bool
		if route.Harness != "pi" || strings.TrimSpace(route.Model) == "" || len(route.Model) > 256 || len(route.ProviderID) > 256 || len(route.Effort) > 32 ||
			len(route.FastMode) > 0 && json.Unmarshal(route.FastMode, &fast) != nil {
			return errors.New("伙伴模型路线无效")
		}
	}
	return nil
}

// defaultModelChain is the chain of every teammate that follows the default.
func (b *chatOneBot) defaultModelChain(configuration ai.Configuration) []oneBotModelRoute {
	if chain, err := b.customModelChain(); err == nil && len(chain) > 0 {
		return chain
	} else if err != nil {
		_ = recordHostEvent(b.control.directory, "onebot:model-chain", err)
	}
	return oneBotDefaultModelChain(configuration, b.control.store.Preferences())
}

func (b *chatOneBot) modelChainSettings(method string, raw json.RawMessage) (any, error) {
	switch method {
	case "modelChain.set":
		var input oneBotModelChainSettings
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		for i := range input.ModelChain {
			if input.ModelChain[i].Harness == "" {
				input.ModelChain[i].Harness = "pi"
			}
		}
		if err := validateOneBotModelChain(input.ModelChain); err != nil {
			return nil, err
		}
		data, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		if err = os.MkdirAll(filepath.Dir(b.modelChainFile()), 0o700); err != nil {
			return nil, err
		}
		if err = settings.WriteAtomic(b.modelChainFile(), data); err != nil {
			return nil, err
		}
	case "modelChain.reset":
		if err := os.Remove(b.modelChainFile()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	custom, err := b.customModelChain()
	if err != nil {
		return nil, err
	}
	if method != "modelChain.get" {
		// Following teammates and invitations waiting for a model see the change.
		b.modelDefaultsChanged()
	}
	if len(custom) > 0 {
		return map[string]any{"modelChain": custom, "isCustomized": true}, nil
	}
	configuration, err := b.control.aiConfiguration()
	if err != nil {
		return nil, err
	}
	return map[string]any{"modelChain": oneBotDefaultModelChain(configuration, b.control.store.Preferences()), "isCustomized": false}, nil
}

// modelDefaultsChanged refreshes companions that follow the default route and
// wakes invitations waiting for a model. It never starts the OneBot process.
func (b *chatOneBot) modelDefaultsChanged() {
	b.resourcesChanged()
	b.startMu.Lock()
	runner := b.runner
	b.startMu.Unlock()
	if runner == nil {
		return
	}
	ctx, cancel := context.WithTimeout(b.control.ctx, 10*time.Second)
	defer cancel()
	if _, err := runner.Call(ctx, "defaults.changed", map[string]any{}); err != nil {
		_ = recordHostEvent(b.control.directory, "onebot:model-defaults", err)
	}
}

// listBots is Cindy local-db:bots:list: profiles with Chat-owned titles, plus
// each canonical chat's latest visible message and unread replies counted from
// the renderer's read positions. The page formats the preview text.
func (b *chatOneBot) listBots(ctx context.Context, runner *chat.Runner, raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		LastReadAtByBotID map[string]float64 `json:"lastReadAtByBotId"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()
	listed, err := runner.Call(ctx, "profiles.list", map[string]any{})
	if err != nil {
		return nil, err
	}
	if err = b.flushRuntimeRefresh(ctx); err != nil {
		return nil, err
	}
	projected, err := b.projectProfiles(listed, true)
	if err != nil {
		return nil, err
	}
	var profiles []map[string]json.RawMessage
	if err = json.Unmarshal(projected, &profiles); err != nil {
		return nil, err
	}
	type item struct {
		BotID      string             `json:"botId"`
		LastReadAt *float64           `json:"lastReadAt,omitempty"`
		Session    *chat.Conversation `json:"session,omitempty"`
	}
	items := make([]item, 0, len(profiles))
	for _, profile := range profiles {
		var botID, canonical string
		_ = json.Unmarshal(profile["id"], &botID)
		_ = json.Unmarshal(profile["canonicalSessionId"], &canonical)
		entry := item{BotID: botID}
		if at, ok := input.LastReadAtByBotID[botID]; ok && at > 0 {
			entry.LastReadAt = &at
		}
		if canonical != "" {
			if conversation, err := b.sessions.store.Conversation(canonical); err == nil {
				entry.Session = &conversation
			}
		}
		items = append(items, entry)
	}
	previewRaw, err := runner.Call(ctx, "bots.previews", map[string]any{"items": items})
	if err != nil {
		return nil, err
	}
	var previews []struct {
		BotID  string  `json:"botId"`
		Text   *string `json:"text"`
		At     *int64  `json:"at"`
		Role   *string `json:"role"`
		Unread int     `json:"unread"`
	}
	if err = json.Unmarshal(previewRaw, &previews); err != nil {
		return nil, err
	}
	for i, preview := range previews {
		if i >= len(profiles) || preview.BotID != items[i].BotID {
			return nil, errors.New("OneBot 预览与档案顺序不一致")
		}
		profiles[i]["lastMessageText"], _ = json.Marshal(preview.Text)
		profiles[i]["lastMessageAt"], _ = json.Marshal(preview.At)
		profiles[i]["lastMessageRole"], _ = json.Marshal(preview.Role)
		profiles[i]["unreadCount"], _ = json.Marshal(preview.Unread)
	}
	return json.Marshal(profiles)
}

// oneBotSessionView is the part of Cindy's Session row that teammate pages read.
func oneBotSessionView(conversation chat.Conversation) map[string]any {
	status := "active"
	if conversation.Archived {
		status = "archived"
	}
	return map[string]any{"id": conversation.ID, "title": conversation.Title, "workingDir": conversation.Directory, "workspaceKind": "dialogue",
		"agentKind": "pi", "status": status, "source": "bot", "model": conversation.Model.Model, "providerId": conversation.Model.Provider,
		"effort": conversation.ThinkingLevel, "permissionMode": conversation.Permission, "fastMode": conversation.FastMode,
		"pinnedAt": nil, "createdAt": conversation.CreatedAt, "updatedAt": conversation.UpdatedAt}
}

// createCanonicalSession is Cindy create-canonical-session: reuse a healthy
// main chat; only a missing/deleted chat receives a replacement under the
// Worker's profile-version and canonical-link checks. Opening a page or
// retrying an invitation must not move the owner's transcript or draft.
func (b *chatOneBot) createCanonicalSession(ctx context.Context, runner *chat.Runner, raw json.RawMessage) (map[string]any, error) {
	var input struct {
		BotID                      string  `json:"botId"`
		ExpectedCanonicalSessionID *string `json:"expectedCanonicalSessionId"`
		ExpectedProfileVersion     int     `json:"expectedProfileVersion"`
		RecoverMissingOnly         bool    `json:"recoverMissingOnly"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if input.ExpectedProfileVersion < 1 {
		return nil, errors.New("expectedProfileVersion 必须是正整数")
	}
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()
	if input.RecoverMissingOnly {
		if input.ExpectedCanonicalSessionID == nil {
			return nil, errors.New("[PRECONDITION_FAILED] Bot 主任务已变化，请刷新后重试")
		}
		if _, err := b.sessions.store.Conversation(*input.ExpectedCanonicalSessionID); err == nil {
			return nil, errors.New("[PRECONDITION_FAILED] Bot 主任务仍然存在，不能按丢失任务恢复")
		}
	}
	value, err := b.contextFor(ctx, map[string]any{"botId": input.BotID, "role": "canonical", "routeKey": "canonical", "sessionCreation": true})
	if err != nil {
		return nil, err
	}
	if value.CanonicalSessionID != "" {
		if current, found := b.sessions.store.Conversation(value.CanonicalSessionID); found == nil {
			// Like Cindy's transaction, a stale pointer returns the current
			// canonical; a matching pointer still checks its profile version.
			if input.ExpectedCanonicalSessionID != nil && *input.ExpectedCanonicalSessionID == current.ID && input.ExpectedProfileVersion != value.ProfileVersion {
				return nil, errors.New("[PRECONDITION_FAILED] Bot 配置已更新，请刷新后重试")
			}
			return map[string]any{"created": false, "canonicalSessionId": current.ID, "session": oneBotSessionView(current)}, nil
		}
	}
	configuration, err := b.control.aiConfiguration()
	if err != nil {
		return nil, err
	}
	value.DefaultModelChain = b.defaultModelChain(configuration)
	plan, err := resolveOneBotModel(value, configuration, chat.Conversation{})
	if err != nil {
		return nil, fmt.Errorf("[PRECONDITION_FAILED] %w", err)
	}
	history := filepath.Join(b.control.directory, "onebot", "history")
	if err = os.MkdirAll(history, 0o700); err != nil {
		return nil, err
	}
	conversation, err := b.sessions.store.NewConversation("", value.Workspace, value.Name, plan.Model, history)
	if err != nil {
		return nil, err
	}
	conversation, err = b.sessions.store.UpdateConversation(conversation.ID, func(c *chat.Conversation) error {
		c.Permission, c.ThinkingLevel, c.FastMode, c.OneBotModelKey = oneBotPermission(value.Capabilities.Permissions), plan.ThinkingLevel, plan.FastMode, plan.Key
		return nil
	})
	if err != nil {
		return nil, err
	}
	replaced, err := runner.Call(ctx, "sessions.replaceCanonical", map[string]any{"botId": input.BotID, "sessionId": conversation.ID,
		"expectedCanonicalSessionId": input.ExpectedCanonicalSessionID, "expectedProfileVersion": input.ExpectedProfileVersion})
	if err != nil {
		// The conversation never became visible: no link owns it yet.
		_ = b.sessions.store.DeleteConversation(conversation.ID, func(directory string) error {
			_, err := recycle.Directory(ctx, directory)
			return err
		})
		return nil, err
	}
	var result struct {
		Archived *string `json:"archivedCanonicalSessionId"`
	}
	if err = json.Unmarshal(replaced, &result); err != nil {
		return nil, err
	}
	if result.Archived != nil && *result.Archived != "" {
		// The previous chat remains readable as history; its runtime stops.
		_ = b.sessions.engine.CloseSession(ctx, *result.Archived)
	}
	b.publishEvent("profile-changed", input.BotID)
	return map[string]any{"created": true, "canonicalSessionId": conversation.ID, "session": oneBotSessionView(conversation)}, nil
}

// botHistory lists the teammate's earlier main chats (Cindy local-db:bots:history).
func (b *chatOneBot) botHistory(ctx context.Context, runner *chat.Runner, raw json.RawMessage) (any, error) {
	var input struct {
		BotID string `json:"botId"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()
	profileRaw, err := runner.Call(ctx, "profiles.get", map[string]string{"botId": input.BotID})
	if err != nil {
		return nil, err
	}
	var profile struct {
		Sessions []struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"sessions"`
	}
	if err = json.Unmarshal(profileRaw, &profile); err != nil {
		return nil, err
	}
	rows := []map[string]any{}
	for _, link := range profile.Sessions {
		if link.Role != "history" {
			continue
		}
		if conversation, err := b.sessions.store.Conversation(link.ID); err == nil {
			rows = append(rows, oneBotSessionView(conversation))
		}
	}
	return rows, nil
}

// serveOneBotBridge answers the Worker's private requests for Cindy's
// invitation and teammate services: Chat records, the auxiliary model, the
// hidden welcome turn, canonical chat creation, model-route readiness and the
// follow-up's conversation activity, metadata and owner reminders.
func (b *chatOneBot) serveOneBotBridge(w http.ResponseWriter, r *http.Request, route string, raw []byte) {
	timeout := 60 * time.Second
	if route == "aux" || route == "aux-stream" {
		timeout = 130 * time.Second
	} else if route == "routine-hook-test" {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	if route == "aux-stream" {
		w.Header().Set("Content-Type", "application/x-ndjson")
		encoder := json.NewEncoder(w)
		result, err := b.auxiliaryTextProgress(ctx, raw, func(text string) error {
			if err := encoder.Encode(map[string]any{"type": "text", "text": text}); err != nil {
				return err
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return nil
		})
		if err != nil {
			result = map[string]any{"ok": false, "reason": "model_error"}
		}
		_ = encoder.Encode(map[string]any{"type": "result", "result": result})
		return
	}
	var value any
	var err error
	switch route {
	case "authorize-routine":
		value, err = b.authorizeRoutine(ctx, raw)
	case "skills-catalog":
		runner, readyErr := b.ensure(ctx)
		if readyErr != nil {
			err = readyErr
			break
		}
		value, err = b.capabilityCatalog(ctx, runner, raw)
	case "group-lane":
		value, err = b.groupLane(ctx, raw)
	case "group-lanes-close":
		value, err = b.closeGroupLanes(ctx, raw)
	case "group-project-dir":
		value, err = b.groupProjectDirectory(raw)
	case "group-folder-recycle":
		value, err = b.recycleGroupFolder(ctx, raw)
	case "group-staged-recycle":
		value, err = b.recycleStagedGroupFolder(ctx, raw)
	case "worktree-state":
		var input struct {
			Directory string `json:"directory"`
		}
		if err = json.Unmarshal(raw, &input); err == nil {
			value, err = chat.ReadWorktreeState(ctx, input.Directory)
		}
	case "lanes":
		value, err = b.laneSessions()
	case "lane-restart", "lane-archive":
		value, err = b.controlLane(ctx, route, raw)
	case "conversation":
		var input struct {
			SessionID string `json:"sessionId"`
		}
		if err = json.Unmarshal(raw, &input); err == nil {
			value, err = b.sessions.store.Conversation(input.SessionID)
		}
	case "aux":
		value, err = b.auxiliaryText(ctx, raw)
	case "voice-resources":
		value, err = b.voiceResources()
    case "creation-resources":
        value, err = b.creationResources()
    case "portrait-catalog":
        value, err = b.portraitCatalog()
    case "portrait-request":
        value, err = b.portraitRequest(ctx,raw)
    case "wechat-authorization":
        value, err = openWechatAuthorization(raw)
	case "voice-record":
		value, err = b.voiceRecord(ctx, raw)
	case "voice-system-audio":
		value, err = b.voiceSystemAudio(raw)
	case "welcome":
		value, err = b.welcomeTurn(ctx, raw)
	case "canonical":
		runner, ensureErr := b.ensure(ctx)
		if ensureErr != nil {
			err = ensureErr
			break
		}
		value, err = b.createCanonicalSession(ctx, runner, raw)
	case "route-check":
		var input struct {
			Capabilities json.RawMessage `json:"capabilities"`
		}
		if err = json.Unmarshal(raw, &input); err == nil {
			value, err = b.routeAvailable(input.Capabilities)
		}
	case "autonomy-activity":
		// The bridge exists only after ensure published the teammate store.
		value = map[string]any{"snapshots": b.activitySnapshots(b.sessions)}
	case "autonomy-sessions":
		value, err = b.autonomySessions(raw)
	case "remind":
		value, err = b.remindOwner(raw)
	case "task-conversation":
		value, err = b.taskConversationFacts(raw)
	case "task-facts":
		value, err = b.taskRuntimeFacts(ctx, raw)
	case "workbench-send":
		value, err = b.workbenchSend(ctx, raw)
	case "routine-hook-test":
		value, err = b.routineHookTest(ctx, raw)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

// routeAvailable reports whether a profile's first model route can run now
// (Cindy canStartWelcome): an invitation waits instead of failing.
func (b *chatOneBot) routeAvailable(capabilities json.RawMessage) (map[string]bool, error) {
	var value oneBotContext
	if err := json.Unmarshal(capabilities, &value.Capabilities); err != nil {
		return nil, err
	}
	configuration, err := b.control.aiConfiguration()
	if err != nil {
		return nil, err
	}
	value.DefaultModelChain = b.defaultModelChain(configuration)
	_, err = resolveOneBotModel(value, configuration, chat.Conversation{})
	return map[string]bool{"available": err == nil}, nil
}

// auxiliaryText is Cindy's auxiliary one-shot: Chat's title model when set,
// otherwise Chat's default model, bounded by the caller's budget.
func (b *chatOneBot) auxiliaryText(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	return b.auxiliaryTextProgress(ctx, raw, nil)
}

func (b *chatOneBot) auxiliaryTextProgress(ctx context.Context, raw json.RawMessage, progress func(string) error) (map[string]any, error) {
	var input struct {
		Prompt               string `json:"prompt"`
		ResponseInstructions string `json:"responseInstructions"`
		TimeoutMs            int    `json:"timeoutMs"`
		MaxTokens            int    `json:"maxTokens"`
		ProviderID           string `json:"providerId"`
		Model                string `json:"model"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Prompt) == "" || len(input.Prompt) > 1<<20 || len(input.ResponseInstructions) > 16<<10 || input.MaxTokens < 0 {
		return map[string]any{"ok": false, "reason": "invalid_request"}, nil
	}
	timeout := time.Duration(input.TimeoutMs) * time.Millisecond
	if timeout <= 0 || timeout > 120*time.Second {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	configuration, err := b.control.aiConfiguration()
	if err != nil {
		return map[string]any{"ok": false, "reason": "no_model"}, nil
	}
	selection := b.control.store.Preferences().TitleModel
	if input.ProviderID != "" && input.Model != "" {
		selection = ai.Selection{Provider: input.ProviderID, Model: input.Model}
	}
	if selection.Provider == "" {
		selection = configuration.Selected("chat")
	}
	if configuration.ValidateSelection(selection) != nil {
		return map[string]any{"ok": false, "reason": "no_model"}, nil
	}
	text, err := b.control.generateWithModelProgress(ctx, selection, ai.Generation{Text: input.Prompt, System: input.ResponseInstructions, MaxTokens: input.MaxTokens, Timeout: timeout}, func(ctx context.Context, generate func(context.Context) (string, error)) (string, error) {
		return generate(ctx)
	}, progress)
	if err != nil {
		return map[string]any{"ok": false, "reason": "model_error"}, nil
	}
	return map[string]any{"ok": true, "text": text}, nil
}

// prepareOwnerInput asks the OneBot store to prepare an owner's message with
// a reply quote or @teammate references (Cindy makerSendTransaction): the
// quoted original is reread from the same chat and each teammate is resolved
// from the roster. The owner's text and the normalised metadata are displayed.
func (b *chatOneBot) prepareOwnerInput(ctx context.Context, sessionID, text string, quote, references json.RawMessage) (string, chat.NativeInputPresentation, error) {
	if len(quote) > 4096 || len(references) > 64*1024 {
		return "", chat.NativeInputPresentation{}, errors.New("引用的消息或伙伴无效")
	}
	runner, err := b.ensure(ctx)
	if err != nil {
		return "", chat.NativeInputPresentation{}, err
	}
	params := map[string]any{"sessionId": sessionID, "text": text}
	if len(quote) > 0 && string(quote) != "null" {
		params["quote"] = quote
	}
	if len(references) > 0 && string(references) != "null" {
		params["agentReferences"] = references
	}
	raw, err := runner.Call(ctx, "tasks.input.prepare", params)
	if err != nil {
		return "", chat.NativeInputPresentation{}, err
	}
	var result struct {
		ModelText       string          `json:"modelText"`
		Quote           json.RawMessage `json:"quote"`
		AgentReferences json.RawMessage `json:"agentReferences"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return "", chat.NativeInputPresentation{}, err
	}
	if result.ModelText == "" {
		return "", chat.NativeInputPresentation{}, errors.New("消息未能准备")
	}
	presentation := chat.NativeInputPresentation{Text: &text}
	if len(result.Quote) > 0 && string(result.Quote) != "null" {
		presentation.ReplyQuote = result.Quote
	}
	if len(result.AgentReferences) > 0 && string(result.AgentReferences) != "null" && string(result.AgentReferences) != "[]" {
		presentation.AgentReferences = result.AgentReferences
	}
	return result.ModelText, presentation, nil
}

// welcomeTurn is Cindy's welcome dispatch: the invitation's greeting request
// enters the canonical chat as a hidden host instruction, with tools off for
// that turn. It carries no owner authority.
func (b *chatOneBot) welcomeTurn(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	var input struct {
		SessionID        string `json:"sessionId"`
		ClientID         string `json:"clientId"`
		Message          string `json:"message"`
		PersistedContent string `json:"persistedContent"`
		ToolsDisabled    bool   `json:"toolsDisabled"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if input.ClientID == "" || len(input.ClientID) > 128 || input.Message == "" || len(input.Message) > 100000 {
		return nil, errors.New("OneBot 欢迎消息无效")
	}
	runner, err := b.ensure(ctx)
	if err != nil {
		return nil, err
	}
	owner, err := runner.Call(ctx, "sessions.lookup", map[string]string{"sessionId": input.SessionID})
	if err != nil {
		return nil, err
	}
	var link struct {
		Role string `json:"role"`
	}
	if err = json.Unmarshal(owner, &link); err != nil || link.Role != "canonical" {
		return nil, errors.New("欢迎消息只能进入伙伴主对话")
	}
	digest := sha256.Sum256([]byte(input.Message))
	if err = b.grantInput(input.ClientID, oneBotInputGrant{SessionID: input.SessionID, ConnectionID: "invitation", Digest: hex.EncodeToString(digest[:])}); err != nil {
		return nil, err
	}
	if _, err = b.sessions.engine.Open(ctx, input.SessionID, 0); err != nil {
		return nil, err
	}
	defer b.sessions.engine.Release(input.SessionID, 0)
	persisted := input.PersistedContent
	if _, err = b.sessions.engine.DispatchPresentedInput(ctx, input.SessionID, input.ClientID, input.Message, "queue",
		chat.NativeInputPresentation{Text: &persisted, Hidden: true, ToolsDisabled: input.ToolsDisabled}); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "clientId": input.ClientID}, nil
}

var oneBotThreadSession = regexp.MustCompile(`^[^:\s]{1,128}$`)
var oneBotThreadClient = regexp.MustCompile(`^\S{1,256}$`)

type oneBotTranscriptStats struct {
	Key             string `json:"key"`
	ReplyCount      int    `json:"replyCount"`
	LastReplyAt     *int64 `json:"lastReplyAt"`
	LastAssistantAt *int64 `json:"lastAssistantAt"`
	HasClient       bool   `json:"hasClient"`
}

func (b *chatOneBot) transcriptStats(ctx context.Context, runner *chat.Runner, items []map[string]any) ([]oneBotTranscriptStats, error) {
	raw, err := runner.Call(ctx, "transcripts.stats", map[string]any{"items": items})
	if err != nil {
		return nil, err
	}
	var stats []oneBotTranscriptStats
	return stats, json.Unmarshal(raw, &stats)
}

// ensureDmThread is Cindy dm-thread:ensure: a private-chat topic is a hidden
// lane of the teammate, opened on a message that is still in its main chat.
func (b *chatOneBot) ensureDmThread(ctx context.Context, runner *chat.Runner, raw json.RawMessage) (map[string]any, error) {
	var input struct {
		BotID         string `json:"botId"`
		RootSessionID string `json:"rootSessionId"`
		RootClientID  string `json:"rootClientId"`
		Title         string `json:"title"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if !oneBotThreadSession.MatchString(input.RootSessionID) || !oneBotThreadClient.MatchString(input.RootClientID) {
		return nil, errors.New("话题的原消息无效")
	}
	title := strings.TrimSpace(input.Title)
	if runes := []rune(title); len(runes) > 120 {
		title = string(runes[:120])
	}
	if title == "" {
		title = "话题"
	}
	routeKey := "dm-thread:" + input.RootSessionID + ":" + input.RootClientID
	unavailable := func(code, message string) (map[string]any, error) {
		return map[string]any{"ok": false, "errorCode": code, "message": message}, nil
	}
	b.sessionMu.Lock()
	found, err := runner.Call(ctx, "sessions.find", map[string]any{"botId": input.BotID, "routeKey": routeKey})
	if err != nil {
		b.sessionMu.Unlock()
		return nil, err
	}
	var existing struct {
		SessionID string `json:"sessionId"`
	}
	_ = json.Unmarshal(found, &existing)
	if existing.SessionID != "" {
		b.sessionMu.Unlock()
		if conversation, err := b.sessions.store.Conversation(existing.SessionID); err == nil && !conversation.Archived {
			return map[string]any{"ok": true, "sessionId": existing.SessionID, "created": false}, nil
		}
		return unavailable("ROOT_UNAVAILABLE", "原消息已不在私聊里")
	}
	ownerRaw, err := runner.Call(ctx, "sessions.lookup", map[string]string{"sessionId": input.RootSessionID})
	var owner struct {
		BotID string `json:"botId"`
		Role  string `json:"role"`
	}
	if err != nil || json.Unmarshal(ownerRaw, &owner) != nil || owner.BotID != input.BotID || owner.Role != "canonical" {
		b.sessionMu.Unlock()
		return unavailable("ROOT_UNAVAILABLE", "原消息已不在私聊里")
	}
	root, err := b.sessions.store.Conversation(input.RootSessionID)
	b.sessionMu.Unlock()
	if err != nil {
		return unavailable("ROOT_UNAVAILABLE", "原消息已不在私聊里")
	}
	stats, err := b.transcriptStats(ctx, runner, []map[string]any{{"key": root.ID, "session": root, "clientId": input.RootClientID}})
	if err != nil {
		return nil, err
	}
	if len(stats) != 1 || !stats[0].HasClient {
		return unavailable("ROOT_UNAVAILABLE", "原消息已不在私聊里")
	}
	conversation, err := b.openRoute(ctx, input.BotID, routeKey, "thread", title)
	if err != nil {
		if strings.Contains(err.Error(), "INACTIVE") {
			return unavailable("BOT_UNAVAILABLE", "伙伴当前不可用")
		}
		if strings.Contains(err.Error(), "模型链") {
			return unavailable("NO_MODEL", "伙伴还没有可用模型")
		}
		return nil, err
	}
	b.publishEvent("profile-changed", input.BotID)
	return map[string]any{"ok": true, "sessionId": conversation.ID, "created": true}, nil
}

// listDmThreads is Cindy dm-thread:list: topics opened on messages of the
// teammate's main chat that have at least one visible message.
func (b *chatOneBot) listDmThreads(ctx context.Context, runner *chat.Runner, raw json.RawMessage) ([]map[string]any, error) {
	var input struct {
		BotID         string `json:"botId"`
		RootSessionID string `json:"rootSessionId"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	b.sessionMu.Lock()
	profileRaw, err := runner.Call(ctx, "profiles.get", map[string]string{"botId": input.BotID})
	if err != nil {
		b.sessionMu.Unlock()
		return nil, err
	}
	var profile struct {
		Sessions []struct {
			ID       string `json:"id"`
			Role     string `json:"role"`
			RouteKey string `json:"routeKey"`
		} `json:"sessions"`
	}
	if err = json.Unmarshal(profileRaw, &profile); err != nil {
		b.sessionMu.Unlock()
		return nil, err
	}
	type topic struct {
		conversation chat.Conversation
		root, client string
	}
	topics := map[string]topic{}
	items := []map[string]any{}
	for _, link := range profile.Sessions {
		rest, ok := strings.CutPrefix(link.RouteKey, "dm-thread:")
		root, client, split := strings.Cut(rest, ":")
		if link.Role != "thread" || !ok || !split || root != input.RootSessionID {
			continue
		}
		conversation, err := b.sessions.store.Conversation(link.ID)
		if err != nil || conversation.Archived {
			continue
		}
		topics[link.ID] = topic{conversation, root, client}
		items = append(items, map[string]any{"key": link.ID, "session": conversation})
	}
	b.sessionMu.Unlock()
	result := []map[string]any{}
	if len(items) == 0 {
		return result, nil
	}
	stats, err := b.transcriptStats(ctx, runner, items)
	if err != nil {
		return nil, err
	}
	for _, stat := range stats {
		value, ok := topics[stat.Key]
		if !ok || stat.ReplyCount == 0 {
			continue
		}
		created, _ := time.Parse(time.RFC3339Nano, value.conversation.CreatedAt)
		result = append(result, map[string]any{"sessionId": stat.Key, "rootSessionId": value.root, "rootClientId": value.client,
			"replyCount": stat.ReplyCount, "lastReplyAt": stat.LastReplyAt, "lastAssistantAt": stat.LastAssistantAt, "createdAt": created.UnixMilli()})
	}
	return result, nil
}

// actTodo is Cindy todos:act: the owner asks the companion to advance one
// agreed next step. The instruction enters its main chat as the owner's input
// with a readable display line; the Todo records whether delivery succeeded.
func (b *chatOneBot) actTodo(ctx context.Context, runner *chat.Runner, raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		BotID     string `json:"botId"`
		ID        string `json:"id"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	requestRaw, err := runner.Call(ctx, "todos.actionRequest", raw)
	if err != nil {
		return nil, err
	}
	var request struct {
		Todo        json.RawMessage `json:"todo"`
		Dispatch    bool            `json:"dispatch"`
		SessionID   string          `json:"sessionId"`
		Message     string          `json:"message"`
		DisplayText string          `json:"displayText"`
	}
	if err = json.Unmarshal(requestRaw, &request); err != nil {
		return nil, err
	}
	if !request.Dispatch {
		return request.Todo, nil
	}
	dispatchErr := func() error {
		digest := sha256.Sum256([]byte(request.Message))
		if err := b.grantInput(input.RequestID, oneBotInputGrant{SessionID: request.SessionID, ConnectionID: "desktop", Owner: true, Digest: hex.EncodeToString(digest[:])}); err != nil {
			return err
		}
		if _, err := b.sessions.engine.Open(ctx, request.SessionID, 0); err != nil {
			return err
		}
		defer b.sessions.engine.Release(request.SessionID, 0)
		_, err := b.sessions.engine.DispatchPresentedInput(ctx, request.SessionID, input.RequestID, request.Message, "queue", chat.NativeInputPresentation{Text: &request.DisplayText})
		return err
	}()
	result := map[string]any{"ok": dispatchErr == nil}
	if dispatchErr != nil {
		// An uncertain delivery is never replayed automatically.
		result["uncertain"], result["error"] = true, dispatchErr.Error()
	}
	settled, err := runner.Call(ctx, "todos.settleAction", map[string]any{"botId": input.BotID, "id": input.ID, "requestId": input.RequestID, "result": result})
	b.publishEvent("workbench-changed", input.BotID)
	if dispatchErr != nil {
		return nil, dispatchErr
	}
	return settled, err
}
