package model

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// Entry is one step of a response's work process: thinking, commentary
// between tool calls, a tool, or an operation (compaction, review,
// approval).
type Entry struct {
	Kind    string // "thinking", "commentary", "tool", "operation"
	Key     string
	At      *float64
	EndedAt *float64
	// Part is a thinking entry's part.
	Part Part
	// Text and Message are a commentary entry's text and the message it
	// is from, its content that text.
	Text      string
	Message   *Message
	Delivered bool
	Tool      Tool
	Operation Operation
}

// Response groups the assistant and tool messages answering one user
// turn: its work process and its answer.
type Response struct {
	Key       string
	Messages  []Message
	Entries   []Entry
	Answer    *Message
	Running   bool
	StartedAt *float64
	// DurationMs is nil until the turn's length is known.
	DurationMs *float64
	HasTools   bool
}

// Item is one row of the transcript: a user message, a presentation-only
// message, or a response.
type Item struct {
	Kind     string // "user", "presentation", "response"
	Key      string
	Message  *Message
	Response *Response
}

func number(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// filesOf reads details.files from a live tool's result.
func filesOf(result any) []ChangedFile {
	object, _ := result.(map[string]any)
	details, _ := object["details"].(map[string]any)
	list, _ := details["files"].([]any)
	var files []ChangedFile
	for _, value := range list {
		f, _ := value.(map[string]any)
		if f == nil {
			continue
		}
		file := ChangedFile{}
		file.Path, _ = f["path"].(string)
		file.Name, _ = f["name"].(string)
		file.Status, _ = f["status"].(string)
		file.Patch, _ = f["patch"].(string)
		if v, ok := f["modified"].(float64); ok {
			file.Modified = &v
		}
		if v, ok := f["size"].(float64); ok {
			file.Size = &v
		}
		files = append(files, file)
	}
	return files
}

// Project groups a conversation into transcript rows, as the React
// window's projectChatProcess: Pi stores one assistant/tool sequence per
// user turn, shown as Cindy's work process. Every public text part is a
// reply; only thinking and actions belong to the disclosures, even when
// text shares a tool-call message.
func Project(messages []Message, liveTools []Tool, running bool, operations []Operation) []*Item {
	var output []*Item
	groups := map[string]*Item{}
	var order []*Item // groups in creation order
	turn := "history"
	response := func(key string) *Response {
		if group, ok := groups[key]; ok {
			return group.Response
		}
		group := &Item{Kind: "response", Key: key, Response: &Response{Key: key}}
		groups[key] = group
		order = append(order, group)
		user := -1
		for i, item := range output {
			if item.Kind == "user" && item.Key == key {
				user = i
				break
			}
		}
		if user >= 0 {
			output = append(output[:user+1], append([]*Item{group}, output[user+1:]...)...)
		} else {
			output = append(output, group)
		}
		return group.Response
	}
	remove := func(target *Item) {
		for i, item := range output {
			if item == target {
				output = append(output[:i], output[i+1:]...)
				return
			}
		}
	}
	for i := range messages {
		message := messages[i]
		if message.PresentationOnly {
			key := first(message.ID, message.TurnID, "presentation-"+strconv.Itoa(i))
			output = append(output, &Item{Kind: "presentation", Key: key, Message: &message})
			continue
		}
		if message.Role == "user" {
			fallback := "user-" + strconv.Itoa(i)
			if message.Timestamp != nil {
				fallback = "user-" + number(*message.Timestamp)
			}
			turn = first(message.TurnID, message.RequestID, message.ID, fallback)
			// Host-authored completion wakes stay in model history, never
			// in the owner's bubble; their replies keep this turn.
			if message.HiddenInput {
				continue
			}
			// A progress batch can expose a partial before the committed
			// user entry: re-anchor it below its owner.
			existing := groups[turn]
			if existing != nil {
				remove(existing)
			}
			output = append(output, &Item{Kind: "user", Key: turn, Message: &message})
			if existing != nil {
				output = append(output, existing)
			}
			continue
		}
		if message.Role != "assistant" && message.Role != "toolResult" {
			continue
		}
		group := response(first(message.TurnID, turn))
		group.Messages = append(group.Messages, message)
	}

	activeTurn := ""
	if running {
		for i := len(operations) - 1; i >= 0; i-- {
			if operations[i].Status == "running" {
				activeTurn = operations[i].TurnID
				break
			}
		}
		if activeTurn == "" {
			for i := len(messages) - 1; i >= 0; i-- {
				if messages[i].Role == "user" && !messages[i].Pending {
					activeTurn = messages[i].TurnID
					break
				}
			}
		}
		if activeTurn == "" {
			activeTurn = turn
		}
		response(activeTurn)
	}
	for _, op := range operations {
		response(op.TurnID)
	}

	live := map[string]Tool{}
	for _, tool := range liveTools {
		live[tool.ID] = tool
	}
	accounted := map[string]bool{}
	var responses []*Response
	for _, item := range output {
		if item.Kind == "response" {
			responses = append(responses, item.Response)
		}
	}
	for _, group := range responses {
		group.Running = running && group.Key == activeTurn
		answer := -1
		last := -1
		for i, m := range group.Messages {
			if m.Role == "assistant" {
				last = i
			}
		}
		if last >= 0 {
			// A started or thinking-only message is not a reply boundary:
			// the answer needs actual prose.
			m := group.Messages[last]
			if (strings.TrimSpace(m.Text()) != "" || m.ErrorMessage != "") && (m.Content.Plain || !m.Content.HasToolCall()) {
				answer = last
				group.Answer = &group.Messages[last]
			}
		}
		results := map[string]Message{}
		var resultOrder []string
		for _, m := range group.Messages {
			if m.Role == "toolResult" {
				if _, ok := results[m.ToolCallID]; !ok {
					resultOrder = append(resultOrder, m.ToolCallID)
				}
				results[m.ToolCallID] = m
			}
		}
		for mi, message := range group.Messages {
			if message.Role != "assistant" {
				continue
			}
			if message.Timestamp != nil && (group.StartedAt == nil || *message.Timestamp < *group.StartedAt) {
				group.StartedAt = message.Timestamp
			}
			for pi, part := range message.Content.List() {
				// The request timestamp anchors a part from streaming to
				// saving: switching to the late ID would remount its
				// disclosure and lose its expansion.
				anchor := strconv.Itoa(mi)
				if message.Timestamp != nil {
					anchor = number(*message.Timestamp)
				} else if message.ID != "" {
					anchor = message.ID
				}
				key := anchor + ":" + strconv.Itoa(pi)
				if part.Type == "thinking" && strings.TrimSpace(part.Thinking) != "" {
					entry := Entry{Kind: "thinking", Key: key, Part: part, At: part.ThinkingStartedAt}
					if entry.At == nil {
						entry.At = message.Timestamp
					}
					if part.ThinkingStartedAt != nil && part.ThinkingDurationMs != nil {
						end := *part.ThinkingStartedAt + *part.ThinkingDurationMs
						entry.EndedAt = &end
					}
					group.Entries = append(group.Entries, entry)
				}
				if part.Type == "text" && mi != answer && part.Text != "" {
					commented := message
					commented.Content = TextContent(part.Text)
					group.Entries = append(group.Entries, Entry{Kind: "commentary", Key: key, Text: part.Text, Message: &commented, Delivered: true, At: message.Timestamp})
				}
				if part.Type == "toolCall" && part.ID != "" {
					result, hasResult := results[part.ID]
					current, hasLive := live[part.ID]
					accounted[part.ID] = true
					group.HasTools = true
					tool := Tool{ID: part.ID, Name: first(part.Name, current.Name, "tool"), Args: part.Arguments}
					if tool.Args == nil {
						tool.Args = current.Args
					}
					if tool.Args == nil {
						tool.Args = map[string]any{}
					}
					tool.Running = !hasResult && group.Running && (!hasLive || current.Running)
					entry := Entry{Kind: "tool", Key: part.ID, At: message.Timestamp}
					if hasResult {
						tool.Result, tool.IsError = result.Text(), result.IsError
						if result.Details != nil {
							tool.Files = result.Details.Files
						}
						entry.EndedAt = result.Timestamp
					} else if hasLive {
						tool.Result, tool.IsError = current.Result, current.IsError
					}
					if tool.Files == nil && hasLive {
						tool.Files = filesOf(current.Result)
					}
					entry.Tool = tool
					group.Entries = append(group.Entries, entry)
				}
			}
		}
		// A page may start after a tool's call: keep its persisted result.
		for _, id := range resultOrder {
			result := results[id]
			if id == "" || accounted[id] {
				continue
			}
			accounted[id] = true
			group.HasTools = true
			group.Entries = append(group.Entries, Entry{Kind: "tool", Key: id, Tool: Tool{ID: id, Name: first(result.ToolName, "tool"),
				Args: map[string]any{}, Result: result.Text(), IsError: result.IsError}})
		}
		if !group.Running && group.StartedAt != nil && last >= 0 {
			m := group.Messages[last]
			if m.Timestamp != nil && m.DurationMs != nil {
				d := math.Max(0, *m.Timestamp+*m.DurationMs-*group.StartedAt)
				group.DurationMs = &d
			}
		}
	}
	for _, tool := range liveTools {
		if accounted[tool.ID] {
			continue
		}
		group := response(first(tool.TurnID, activeTurn, turn))
		tool.Running = running && tool.Running
		group.Entries = append(group.Entries, Entry{Kind: "tool", Key: tool.ID, Tool: tool})
		group.HasTools = true
	}
	for _, op := range operations {
		group := response(op.TurnID)
		if op.Kind == "turn" {
			started := op.StartedAt
			group.StartedAt, group.DurationMs = &started, op.DurationMs
		} else if op.Kind != "retry" {
			// Retry attempts are session status, not performed work.
			started := op.StartedAt
			group.Entries = append(group.Entries, Entry{Kind: "operation", Key: op.ID, At: &started, Operation: op})
		}
	}
	for _, item := range order {
		group := item.Response
		for _, op := range operations {
			if op.TurnID == group.Key && op.Kind == "turn" && op.Status == "running" {
				if group.Running {
					started := op.StartedAt
					group.StartedAt = &started
				}
				break
			}
		}
		sort.SliceStable(group.Entries, func(i, j int) bool { return at(group.Entries[i]) < at(group.Entries[j]) })
	}
	return output
}

func at(e Entry) float64 {
	if e.At == nil {
		return math.MaxFloat64
	}
	return *e.At
}
