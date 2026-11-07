// Package model is the Chat page's view of a conversation: the Host's
// chat.state snapshots and chat.poll events, folded into one state the
// page draws. It ports the React window's chat-state.ts and
// chat-process.ts (WinOne, after Cindy's work-process grouping) so both
// windows read the same Host stream the same way.
package model

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Permission is how a conversation's tools are approved.
type Permission string

const (
	PermissionAsk    Permission = "ask"
	PermissionAuto   Permission = "auto"
	PermissionBypass Permission = "bypassPermissions"
)

// Part is one piece of a message: text, thinking, a tool call or image metadata.
type Part struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	Thinking  string         `json:"thinking,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	// Tool images use the Host-owned, read-only attachment path. Data is kept
	// only when present in a compatible wire payload, never synthesized here.
	MimeType string `json:"mimeType,omitempty"`
	Path     string `json:"path,omitempty"`
	Data     string `json:"data,omitempty"`
	// ThinkingStartedAt and ThinkingDurationMs time a thinking part, in
	// milliseconds since the epoch and milliseconds.
	ThinkingStartedAt  *float64 `json:"thinkingStartedAt,omitempty"`
	ThinkingDurationMs *float64 `json:"thinkingDurationMs,omitempty"`
}

// Content is a message's content: the Host sends either plain text or a
// list of parts, and the page keeps which.
type Content struct {
	Plain bool
	Text  string
	Parts []Part
}

// TextContent is plain-text content.
func TextContent(text string) Content { return Content{Plain: true, Text: text} }

func (c *Content) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '"' {
		*c = Content{Plain: true}
		return json.Unmarshal(data, &c.Text)
	}
	*c = Content{}
	if string(data) == "null" {
		return nil
	}
	return json.Unmarshal(data, &c.Parts)
}

func (c Content) MarshalJSON() ([]byte, error) {
	if c.Plain {
		return json.Marshal(c.Text)
	}
	if c.Parts == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(c.Parts)
}

// List returns the content as parts, plain text as one text part. The
// parts are a copy the caller may change.
func (c Content) List() []Part {
	if c.Plain {
		return []Part{{Type: "text", Text: c.Text}}
	}
	return append([]Part(nil), c.Parts...)
}

// HasToolCall reports whether the content calls a tool.
func (c Content) HasToolCall() bool {
	for _, part := range c.Parts {
		if part.Type == "toolCall" {
			return true
		}
	}
	return false
}

// ChangedFile is a file a tool changed.
type ChangedFile struct {
	Path     string   `json:"path"`
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	Modified *float64 `json:"modified,omitempty"`
	Size     *float64 `json:"size,omitempty"`
	Patch    string   `json:"patch,omitempty"`
}

// Details carries what a tool result reports beside its text.
type Details struct {
	Files []ChangedFile `json:"files,omitempty"`
}

// Attachment is a file sent with a message.
type Attachment struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Size     float64 `json:"size"`
	MimeType string  `json:"mimeType"`
	// Annotation and ImageAnnotation are the browser comment and image
	// strokes a file came with; the page passes them on unread.
	Annotation      json.RawMessage `json:"annotation,omitempty"`
	ImageAnnotation json.RawMessage `json:"imageAnnotation,omitempty"`
}

// DeliveryMode is how queued input reaches a running turn.
type DeliveryMode string

const (
	DeliveryQueue DeliveryMode = "queue"
	DeliverySteer DeliveryMode = "steer"
)

// Message is one entry of a conversation.
type Message struct {
	ID               string `json:"id,omitempty"`
	PresentationOnly bool   `json:"presentationOnly,omitempty"`
	// AgentMeta (voice calls, bot collaboration, direct messages, starter
	// options, learning) and ReplyQuote are read by the bot views.
	AgentMeta    json.RawMessage `json:"agentMeta,omitempty"`
	HiddenInput  bool            `json:"hiddenInput,omitempty"`
	ReplyQuote   json.RawMessage `json:"replyQuote,omitempty"`
	Pending      bool            `json:"pending,omitempty"`
	Queued       bool            `json:"queued,omitempty"`
	DeliveryMode DeliveryMode    `json:"deliveryMode,omitempty"`
	Role         string          `json:"role"`
	Content      Content         `json:"content"`
	// Timestamp is milliseconds since the epoch; nil when the Host sent none.
	Timestamp    *float64     `json:"timestamp,omitempty"`
	DurationMs   *float64     `json:"durationMs,omitempty"`
	TurnID       string       `json:"turnId,omitempty"`
	ToolCallID   string       `json:"toolCallId,omitempty"`
	ToolName     string       `json:"toolName,omitempty"`
	IsError      bool         `json:"isError,omitempty"`
	ErrorMessage string       `json:"errorMessage,omitempty"`
	Model        string       `json:"model,omitempty"`
	Provider     string       `json:"provider,omitempty"`
	ProviderName string       `json:"providerName,omitempty"`
	ModelName    string       `json:"modelName,omitempty"`
	RequestID    string       `json:"requestId,omitempty"`
	StopReason   string       `json:"stopReason,omitempty"`
	Details      *Details     `json:"details,omitempty"`
	Attachments  []Attachment `json:"attachments,omitempty"`
}

// Text joins the message's text parts, as the React window's messageText.
func (m Message) Text() string {
	if m.Content.Plain {
		return m.Content.Text
	}
	var texts []string
	for _, part := range m.Content.Parts {
		if part.Type == "text" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// Question is one question of an approval asking the user.
type Question struct {
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
}

// Approval is a tool waiting for the user's permission, or (Kind
// "question") a question the agent asks.
type Approval struct {
	Kind                  string         `json:"kind,omitempty"`
	Questions             []Question     `json:"questions,omitempty"`
	ID                    string         `json:"id"`
	Tool                  string         `json:"tool"`
	Args                  map[string]any `json:"args"`
	Cwd                   string         `json:"cwd,omitempty"`
	Outside               bool           `json:"outside,omitempty"`
	ProtectedFile         bool           `json:"protectedFile,omitempty"`
	ScopeLabel            string         `json:"scopeLabel,omitempty"`
	AutoReviewUnavailable bool           `json:"autoReviewUnavailable,omitempty"`
	Reason                string         `json:"reason,omitempty"`
}

// Operation is a unit of a turn the Host times: the turn itself,
// compaction, a retry, a review or an approval.
type Operation struct {
	ID          string   `json:"id"`
	TurnID      string   `json:"turnId"`
	Kind        string   `json:"kind"`
	StartedAt   float64  `json:"startedAt"`
	DurationMs  *float64 `json:"durationMs,omitempty"`
	Status      string   `json:"status"`
	Attempt     *float64 `json:"attempt,omitempty"`
	MaxAttempts *float64 `json:"maxAttempts,omitempty"`
	DelayMs     *float64 `json:"delayMs,omitempty"`
}

// Usage is the conversation's token use and cost.
type Usage struct {
	Input       float64 `json:"input"`
	Output      float64 `json:"output"`
	CacheRead   float64 `json:"cacheRead"`
	CacheWrite  float64 `json:"cacheWrite"`
	TotalTokens float64 `json:"totalTokens"`
	Cost        float64 `json:"cost"`
	CostKnown   bool    `json:"costKnown"`
}

// ContextUsage is how full the model's context is; Percent is nil when
// the window is unknown.
type ContextUsage struct {
	Tokens        float64  `json:"tokens"`
	ContextWindow float64  `json:"contextWindow"`
	Percent       *float64 `json:"percent"`
}

// SubagentResult is what a finished subagent answered.
type SubagentResult struct {
	Status string `json:"status,omitempty"`
	Answer string `json:"answer,omitempty"`
}

// Subagent is an agent the conversation started.
type Subagent struct {
	Name           string          `json:"name"`
	Role           string          `json:"role,omitempty"`
	Status         string          `json:"status"`
	Task           string          `json:"task,omitempty"`
	Model          string          `json:"model,omitempty"`
	Background     bool            `json:"background,omitempty"`
	ConversationID string          `json:"conversationId,omitempty"`
	Result         *SubagentResult `json:"result,omitempty"`
	CreatedAt      *float64        `json:"createdAt,omitempty"`
	FinishedAt     *float64        `json:"finishedAt,omitempty"`
}

// WorkflowItem is a step of the agent's plan.
type WorkflowItem struct {
	Step   string `json:"step"`
	Status string `json:"status"` // pending, in_progress, completed
}

// Workflow is the agent's plan.
type Workflow struct {
	Items       []WorkflowItem `json:"items"`
	Explanation string         `json:"explanation,omitempty"`
}

// Task is a scheduled unit of durable work.
type Task struct {
	ID             float64  `json:"id"`
	Kind           string   `json:"kind"`
	ConversationID float64  `json:"conversationId"`
	Owner          *float64 `json:"owner,omitempty"`
	Background     bool     `json:"background"`
	State          string   `json:"state"`
}

// TaskGraph is the conversation's durable tasks.
type TaskGraph struct {
	Scheduling string  `json:"scheduling"`
	Tasks      []Task  `json:"tasks"`
	Total      float64 `json:"total"`
}

// Snapshot is a conversation as chat.state and chat.open return it.
type Snapshot struct {
	Messages          []Message     `json:"messages"`
	Operations        []Operation   `json:"operations,omitempty"`
	HasOlder          bool          `json:"hasOlder"`
	Before            string        `json:"before,omitempty"`
	ModelError        string        `json:"modelError,omitempty"`
	Streaming         bool          `json:"streaming,omitempty"`
	Notice            string        `json:"notice,omitempty"`
	Usage             *Usage        `json:"usage,omitempty"`
	ContextUsage      *ContextUsage `json:"contextUsage,omitempty"`
	Subagents         []Subagent    `json:"subagents,omitempty"`
	Workflow          *Workflow     `json:"workflow,omitempty"`
	BackgroundRunning bool          `json:"backgroundRunning,omitempty"`
	Paused            *bool         `json:"paused,omitempty"`
	QueuePaused       *bool         `json:"queuePaused,omitempty"`
	TaskGraph         *TaskGraph    `json:"taskGraph,omitempty"`
}

// Tool is a tool running or run in the live turn.
type Tool struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Args        map[string]any `json:"args"`
	Running     bool           `json:"running"`
	ResultParts []Part         `json:"resultParts,omitempty"`
	Result      any            `json:"result,omitempty"`
	IsError     bool           `json:"isError,omitempty"`
	TurnID      string         `json:"turnId,omitempty"`
	Files       []ChangedFile  `json:"files,omitempty"`
}

// State is what the page draws for the open conversation.
type State struct {
	Snapshot
	Running   bool
	Status    string
	Approvals []Approval
	Tools     []Tool
	Reviewing bool
}

// Empty is the state before anything loaded.
func Empty() State { return State{Status: "idle"} }

// FromSnapshot is a conversation's state from a snapshot, nothing live.
func FromSnapshot(s Snapshot) State {
	state := Empty()
	state.Snapshot = s
	return state
}

// Event is one event of chat.poll.
type Event struct {
	Type              string         `json:"type"`
	ID                string         `json:"id,omitempty"`
	RequestID         string         `json:"requestId,omitempty"`
	TurnID            string         `json:"turnId,omitempty"`
	Operation         *Operation     `json:"operation,omitempty"`
	Request           *Approval      `json:"request,omitempty"`
	EventType         string         `json:"eventType,omitempty"`
	Message           *Message       `json:"message,omitempty"`
	Kind              string         `json:"kind,omitempty"`
	Index             *int           `json:"index,omitempty"`
	Text              string         `json:"text,omitempty"`
	ToolCallID        string         `json:"toolCallId,omitempty"`
	ToolName          string         `json:"toolName,omitempty"`
	Args              map[string]any `json:"args,omitempty"`
	Result            any            `json:"result,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
	Status            string         `json:"status,omitempty"`
	Error             string         `json:"error,omitempty"`
	State             *Snapshot      `json:"state,omitempty"`
	Running           bool           `json:"running,omitempty"`
	Timestamp         *float64       `json:"timestamp,omitempty"`
	StartedAt         *float64       `json:"startedAt,omitempty"`
	DurationMs        *float64       `json:"durationMs,omitempty"`
	Usage             *Usage         `json:"usage,omitempty"`
	ContextUsage      *ContextUsage  `json:"contextUsage,omitempty"`
	Subagents         []Subagent     `json:"subagents,omitempty"`
	Workflow          *Workflow      `json:"workflow,omitempty"`
	BackgroundRunning bool           `json:"backgroundRunning,omitempty"`
	Paused            *bool          `json:"paused,omitempty"`
	QueuePaused       *bool          `json:"queuePaused,omitempty"`
	TaskGraph         *TaskGraph     `json:"taskGraph,omitempty"`
	Before            *float64       `json:"before,omitempty"`
	After             *float64       `json:"after,omitempty"`
}

// Poll is a page of chat.poll: the events after the cursor and the
// conversation's live state.
type Poll struct {
	Events []struct {
		Sequence float64 `json:"sequence"`
		Data     Event   `json:"data"`
	} `json:"events"`
	Cursor    float64    `json:"cursor"`
	Reset     bool       `json:"reset,omitempty"`
	More      bool       `json:"more,omitempty"`
	Running   bool       `json:"running"`
	Status    string     `json:"status"`
	Error     string     `json:"error,omitempty"`
	Approvals []Approval `json:"approvals"`
}

// MergeOperations keeps one record per operation, the newer winning.
func MergeOperations(older, newer []Operation) []Operation {
	var out []Operation
	at := map[string]int{}
	for _, list := range [][]Operation{older, newer} {
		for _, op := range list {
			if i, ok := at[op.ID]; ok {
				out[i] = op
				continue
			}
			at[op.ID] = len(out)
			out = append(out, op)
		}
	}
	return out
}

func sameTime(a, b *float64) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

// sameMessage identifies a message by its committed input receipts, not
// arrival order or text: a steer can overtake queued input, and a saved
// assistant ID may arrive after its timestamp-only partial. Pending rows
// never split a live response.
func sameMessage(a, b Message) bool {
	if a.Role != b.Role {
		return false
	}
	if a.ID != "" && b.ID != "" && a.ID == b.ID {
		return true
	}
	if a.Role == "user" && a.RequestID != "" && b.RequestID != "" {
		return a.RequestID == b.RequestID
	}
	return !a.Pending && !b.Pending && a.Timestamp != nil && sameTime(a.Timestamp, b.Timestamp) &&
		a.ToolCallID == b.ToolCallID && (a.TurnID == "" || b.TurnID == "" || a.TurnID == b.TurnID)
}

// Upsert places incoming among messages: it replaces the same committed
// message (keeping a known ID), or is inserted before the first pending
// row, a user row before its turn's first reply.
func Upsert(current []Message, incoming Message) []Message {
	messages := append([]Message(nil), current...)
	index := -1
	for i, m := range messages {
		if sameMessage(m, incoming) {
			index = i
			break
		}
	}
	if index >= 0 && !messages[index].Pending {
		if incoming.ID == "" {
			incoming.ID = messages[index].ID
		}
		messages[index] = incoming
		return messages
	}
	if index >= 0 {
		messages = append(messages[:index], messages[index+1:]...)
	}
	before := -1
	if !incoming.Pending {
		for i, m := range messages {
			if m.Pending {
				before = i
				break
			}
		}
	}
	if incoming.Role == "user" && incoming.TurnID != "" {
		for i, m := range messages {
			if m.Role != "user" && m.TurnID == incoming.TurnID {
				if before < 0 || i < before {
					before = i
				}
				break
			}
		}
	}
	if before < 0 {
		before = len(messages)
	}
	messages = append(messages[:before], append([]Message{incoming}, messages[before:]...)...)
	return messages
}

// Apply folds one event into the state, as the React window's
// applyChatEvent.
func Apply(state State, event Event) State {
	switch event.Type {
	case "queue_changed":
		if event.State == nil {
			return state
		}
		var saved []Message
		ids := map[string]bool{}
		for _, m := range state.Messages {
			if !m.Pending {
				saved = append(saved, m)
				ids[m.RequestID] = true
			}
		}
		for _, m := range event.State.Messages {
			if m.Pending && !ids[m.RequestID] {
				saved = append(saved, m)
			}
		}
		state.QueuePaused = event.State.QueuePaused
		state.Messages = saved
		return state
	case "notice":
		state.Notice = event.Text
		return state
	case "usage":
		if event.Usage != nil {
			state.Usage = event.Usage
		}
		if event.ContextUsage != nil {
			state.ContextUsage = event.ContextUsage
		}
		return state
	case "durable_state":
		state.Subagents = event.Subagents
		if event.Workflow != nil {
			state.Workflow = event.Workflow
		}
		state.BackgroundRunning = event.BackgroundRunning
		if event.Paused != nil {
			state.Paused = event.Paused
		}
		if event.TaskGraph != nil {
			state.TaskGraph = event.TaskGraph
		}
		return state
	case "resumed", "branch_reset":
		if event.State == nil {
			return state
		}
		next := FromSnapshot(*event.State)
		next.Running = event.State.Streaming
		next.Status = "idle"
		if next.Running {
			next.Status = "running"
		}
		return next
	case "operation":
		if event.Operation != nil {
			state.Operations = MergeOperations(state.Operations, []Operation{*event.Operation})
		}
		return state
	case "approval":
		if event.Request == nil {
			return state
		}
		var approvals []Approval
		for _, a := range state.Approvals {
			if a.ID != event.Request.ID {
				approvals = append(approvals, a)
			}
		}
		state.Approvals = append(approvals, *event.Request)
		return state
	case "approval_cancelled":
		var approvals []Approval
		for _, a := range state.Approvals {
			if a.ID != event.ID {
				approvals = append(approvals, a)
			}
		}
		state.Approvals = approvals
		return state
	case "review":
		state.Reviewing = event.Running
		return state
	case "thinking_timing":
		return thinkingTiming(state, event)
	case "settled":
		if event.State == nil {
			return state
		}
		next := FromSnapshot(*event.State)
		next.Status = event.Status
		if next.Status == "" {
			next.Status = "completed"
		}
		return next
	case "delta":
		return delta(state, event)
	case "pi_event":
		if (event.EventType == "message_start" || event.EventType == "message_end") && event.Message != nil {
			state.Messages = Upsert(state.Messages, *event.Message)
			return state
		}
		if strings.HasPrefix(event.EventType, "tool_execution_") {
			return toolEvent(state, event)
		}
	}
	return state
}

func index(event Event) int {
	if event.Index != nil && *event.Index >= 0 {
		return *event.Index
	}
	return 0
}

func thinkingTiming(state State, event Event) State {
	messages := append([]Message(nil), state.Messages...)
	for i, m := range messages {
		if m.Role != "assistant" || !sameTime(m.Timestamp, event.Timestamp) {
			continue
		}
		content := m.Content.List()
		at := index(event)
		for len(content) <= at {
			content = append(content, Part{Type: "thinking"})
		}
		content[at].ThinkingStartedAt, content[at].ThinkingDurationMs = event.StartedAt, event.DurationMs
		m.Content = Content{Parts: content}
		messages[i] = m
	}
	state.Messages = messages
	return state
}

// delta appends streamed text or thinking to the live answer of its
// turn, starting one (before any pending row) when none is there yet.
func delta(state State, event Event) State {
	messages := append([]Message(nil), state.Messages...)
	at := len(messages) - 1
	for at >= 0 && (messages[at].Role != "assistant" ||
		(event.TurnID != "" && messages[at].TurnID != event.TurnID) ||
		(event.Timestamp != nil && !sameTime(messages[at].Timestamp, event.Timestamp))) {
		at--
	}
	if at < 0 {
		at = len(messages)
		for i, m := range messages {
			if m.Pending {
				at = i
				break
			}
		}
		live := Message{Role: "assistant", Content: Content{Parts: []Part{}}, TurnID: event.TurnID, Timestamp: event.Timestamp}
		messages = append(messages[:at], append([]Message{live}, messages[at:]...)...)
	}
	current := messages[at]
	content := current.Content.List()
	part := index(event)
	for len(content) <= part {
		content = append(content, Part{Type: "text"})
	}
	if event.Kind == "thinking_delta" {
		content[part].Type = "thinking"
		content[part].Thinking += event.Text
		content[part].Text = ""
	} else {
		content[part] = Part{Type: "text", Text: content[part].Text + event.Text}
	}
	current.Content = Content{Parts: content}
	messages[at] = current
	state.Messages = messages
	state.Running = true
	return state
}

// The SDK's live result has the same content-parts contract as a stored
// toolResult message. Preserve that list before the text-only row projection.
func resultParts(result any) []Part {
	object, ok := result.(map[string]any)
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(object["content"])
	if err != nil {
		return nil
	}
	var content Content
	if json.Unmarshal(encoded, &content) != nil {
		return nil
	}
	return content.List()
}

func toolEvent(state State, event Event) State {
	tools := append([]Tool(nil), state.Tools...)
	at := -1
	for i, t := range tools {
		if t.ID == event.ToolCallID {
			at = i
			break
		}
	}
	var tool Tool
	if at < 0 {
		tool = Tool{ID: event.ToolCallID, Name: event.ToolName, Args: event.Args}
		if tool.Name == "" {
			tool.Name = "tool"
		}
		if tool.Args == nil {
			tool.Args = map[string]any{}
		}
	} else {
		tool = tools[at]
	}
	if event.TurnID != "" {
		tool.TurnID = event.TurnID
	}
	tool.Running = event.EventType != "tool_execution_end"
	tool.Result, tool.IsError = event.Result, event.IsError
	tool.ResultParts = resultParts(event.Result)
	if at < 0 {
		tools = append(tools, tool)
	} else {
		tools[at] = tool
	}
	state.Tools = tools
	return state
}
