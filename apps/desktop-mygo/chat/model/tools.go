package model

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ToolDescriptor is a tool call read for its row, as Cindy's
// describeToolUse (shared/toolUseDescriptor.ts, Apache-2.0): which kind of
// work it is and the argument the row names. It holds data, not wording;
// the views translate it.
type ToolDescriptor struct {
	// Kind is "command", "file", "fileChange", "search", "web", "todo",
	// "task", "mcp", "dynamic", "collab" or "generic".
	Kind     string
	ToolName string
	// Action is a file tool's "read", "edit" or "create".
	Action             string
	FilePath, FileName string
	// Command and Description are a command's; Description is the model's
	// own one-line summary, when it wrote one.
	Command, Description string
	// Pattern is a search's; Target a web fetch's URL or search's query.
	Pattern, Target string
	// ToolLabel and Detail name an MCP, dynamic or collab tool and the
	// argument that says most about the call.
	ToolLabel, Detail string
	// CreatedPath is the file an MCP tool made (its outPath argument).
	CreatedPath string
}

// detailKeys are the arguments that describe an MCP or generic call, in
// order.
var detailKeys = []string{"description", "url", "query", "path", "file_path", "title", "name", "prompt"}

var underscores = regexp.MustCompile(`_+`)

func humanizeToolToken(token string) string {
	return strings.Join(strings.Fields(underscores.ReplaceAllString(token, " ")), " ")
}

// truncateToolText shortens text to max characters with "...", as
// Cindy's truncateToolText.
func truncateToolText(text string, max int) string {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	runes := []rune(text)
	return strings.TrimRight(string(runes[:max-3]), " \t\r\n") + "..."
}

func nonEmpty(input map[string]any, key string) string {
	s, _ := input[key].(string)
	return strings.TrimSpace(s)
}

func detailOf(input map[string]any) string {
	for _, key := range detailKeys {
		if v := nonEmpty(input, key); v != "" {
			return truncateToolText(v, 80)
		}
	}
	return ""
}

// parseToolName splits mcp__server__tool, mcp:server:tool, dynamic:ns:tool
// and collab:tool names; kind is "" for a plain name.
func parseToolName(name string) (kind, server, tool string) {
	switch {
	case strings.HasPrefix(name, "mcp__"):
		segments := strings.Split(name[len("mcp__"):], "__")
		if len(segments) >= 2 && segments[0] != "" {
			if tool := strings.Join(segments[1:], "__"); tool != "" {
				return "mcp", segments[0], tool
			}
		}
	case strings.HasPrefix(name, "mcp:"):
		segments := strings.Split(name, ":")
		if len(segments) >= 3 && segments[1] != "" {
			if tool := strings.Join(segments[2:], ":"); tool != "" {
				return "mcp", segments[1], tool
			}
		}
	case strings.HasPrefix(name, "dynamic:"):
		segments := strings.Split(name, ":")
		if len(segments) >= 3 && segments[1] != "" && strings.Join(segments[2:], ":") != "" {
			return "dynamic", segments[1], strings.Join(segments[2:], ":")
		}
		if len(segments) == 2 && segments[1] != "" {
			return "dynamic", "", segments[1]
		}
	case strings.HasPrefix(name, "collab:"):
		if tool := name[len("collab:"):]; tool != "" {
			return "collab", "", tool
		}
	}
	return "", "", ""
}

// DescribeTool reads a tool call. Pi's MCP gateway (cindy_mcp_call_tool)
// is shown as the server's tool it calls. Command intents (Cindy's
// commandIntent rules that name `cat a.go` as reading a.go) are not
// ported: a command row shows the command itself, which is what those
// rules fall back to whenever a command may have side effects.
func DescribeTool(name string, input map[string]any) ToolDescriptor {
	if name == "cindy_mcp_call_tool" {
		server, tool := nonEmpty(input, "server"), nonEmpty(input, "tool")
		args, isObject := input["args"].(map[string]any)
		if server != "" && tool != "" && (input["args"] == nil || isObject) {
			name, input = "mcp:"+server+":"+tool, args
			if input == nil {
				input = map[string]any{}
			}
		}
	}
	d := ToolDescriptor{ToolName: name}
	if kind, _, tool := parseToolName(name); kind != "" {
		d.Kind, d.ToolLabel, d.Detail = kind, humanizeToolToken(tool), detailOf(input)
		if kind == "mcp" {
			for _, key := range []string{"outPath", "out_path", "outputPath", "output_path"} {
				if v := nonEmpty(input, key); v != "" {
					d.CreatedPath = v
					break
				}
			}
		}
		return d
	}
	file := func(action string) ToolDescriptor {
		path := nonEmpty(input, "file_path")
		if path == "" {
			path = nonEmpty(input, "path")
		}
		if path == "" {
			return ToolDescriptor{Kind: "generic", ToolName: name, Detail: detailOf(input)}
		}
		fileName := basename(path)
		if fileName == "" {
			fileName = path
		}
		return ToolDescriptor{Kind: "file", ToolName: name, Action: action, FilePath: path, FileName: fileName}
	}
	switch name {
	case "Bash", "bash":
		d.Kind, d.Description, d.Command = "command", nonEmpty(input, "description"), nonEmpty(input, "command")
	case "exec":
		d.Kind, d.Command = "command", nonEmpty(input, "displayCommand")
		if d.Command == "" {
			d.Command = nonEmpty(input, "command")
		}
	case "Read", "read", "ls":
		return file("read")
	case "Edit", "MultiEdit", "edit":
		return file("edit")
	case "Write", "write":
		return file("create")
	case "Grep", "Glob", "grep", "find":
		if d.Pattern = nonEmpty(input, "pattern"); d.Pattern == "" {
			d.Kind, d.Detail = "generic", detailOf(input)
		} else {
			d.Kind = "search"
		}
	case "WebFetch", "WebSearch", "web_search":
		key := "query"
		if name == "WebFetch" {
			key = "url"
		}
		if d.Target = nonEmpty(input, key); d.Target == "" {
			d.Kind, d.Detail = "generic", detailOf(input)
		} else {
			d.Kind = "web"
		}
	case "TodoWrite", "update_plan":
		d.Kind = "todo"
	case "Task", "Agent":
		d.Kind, d.Description = "task", nonEmpty(input, "description")
	case "file_change":
		d.Kind = "fileChange"
	default:
		d.Kind, d.Detail = "generic", detailOf(input)
	}
	return d
}

// DisplayParam is the argument a tool row names after its verb, as
// AgentActionRow's displayParam.
func (d ToolDescriptor) DisplayParam(input map[string]any) string {
	switch d.Kind {
	case "file":
		return d.FileName
	case "command":
		return first(d.Description, d.Command)
	case "search":
		return d.Pattern
	case "web":
		return d.Target
	case "mcp":
		return first(d.Detail, d.ToolLabel)
	}
	for _, key := range []string{"path", "command", "query"} {
		if v, ok := input[key]; ok && v != nil {
			if s, ok := v.(string); ok {
				return s
			}
			data, _ := json.Marshal(v)
			return string(data)
		}
	}
	return ""
}

// IsFileTool reports a read, write or edit of one file, whose row opens
// the operation's snapshot rather than its raw data.
func (d ToolDescriptor) IsFileTool() bool {
	if d.Kind != "file" || d.FilePath == "" {
		return false
	}
	switch d.ToolName {
	case "Read", "Write", "Edit", "MultiEdit", "read", "write", "edit":
		return true
	}
	return false
}

// ToolResultText is a tool result's text: a string as it is, or the text
// parts of an MCP-style {content: [...]} or {output: [...]} envelope. A
// string is never parsed as JSON: a file may hold a "content" field.
func ToolResultText(result any) (string, bool) {
	switch v := result.(type) {
	case string:
		return v, true
	case map[string]any:
		list, ok := v["content"].([]any)
		if !ok {
			list, ok = v["output"].([]any)
		}
		if !ok {
			return "", false
		}
		var texts []string
		for _, item := range list {
			part, _ := item.(map[string]any)
			if text, isText := part["text"].(string); isText && part["type"] == "text" {
				texts = append(texts, text)
			}
		}
		return strings.Join(texts, "\n"), true
	}
	return "", false
}

// FileSnapshot is what a file tool read, wrote or changed, as the tool
// ran: Kind "document" or "diff", Source "read", "write" or "edit".
type FileSnapshot struct {
	Kind, Source, Text string
}

func textOf(entry map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if v, ok := entry[key]; ok && v != nil {
			s, isString := v.(string)
			return s, isString
		}
	}
	return "", false
}

// ToolFileSnapshot is a file tool's snapshot, as chat-tool-content.ts's
// toolFileContent: a read's result, a write's content, or an edit's
// replacements as a diff, one hunk per replacement.
func ToolFileSnapshot(name string, input map[string]any, result any) (FileSnapshot, bool) {
	switch strings.ToLower(name) {
	case "read":
		text, ok := ToolResultText(result)
		return FileSnapshot{Kind: "document", Source: "read", Text: text}, ok
	case "write":
		text, ok := input["content"].(string)
		return FileSnapshot{Kind: "document", Source: "write", Text: text}, ok
	case "edit", "multiedit":
		entries := []any{input}
		if list, ok := input["edits"].([]any); ok {
			entries = list
		}
		var parts []string
		for i, raw := range entries {
			entry, _ := raw.(map[string]any)
			if entry == nil {
				continue
			}
			oldText, okOld := textOf(entry, "oldText", "old_string", "oldString")
			newText, okNew := textOf(entry, "newText", "new_string", "newString")
			if !okOld || !okNew {
				continue
			}
			parts = append(parts, "@@ "+strconv.Itoa(i+1)+" @@\n"+prefixLines(oldText, "-")+"\n"+prefixLines(newText, "+"))
		}
		if len(parts) > 0 {
			return FileSnapshot{Kind: "diff", Source: "edit", Text: strings.Join(parts, "\n")}, true
		}
	}
	return FileSnapshot{}, false
}

func prefixLines(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

var planStatuses = map[string]bool{"pending": true, "in_progress": true, "completed": true}

// WorkflowFromTool is the plan an update_plan call published, as
// chat-plan-state.ts: only a finished, successful call counts; the plan
// is its saved result's items, else its arguments'.
func WorkflowFromTool(tool Tool) *Workflow {
	if tool.Name != "update_plan" || tool.Running || tool.IsError || tool.Result == nil {
		return nil
	}
	var saved map[string]any
	if text, ok := ToolResultText(tool.Result); ok {
		_ = json.Unmarshal([]byte(text), &saved)
	}
	items, ok := saved["items"]
	if !ok || items == nil {
		items = tool.Args["plan"]
	}
	list, isList := items.([]any)
	if !isList {
		return nil
	}
	w := &Workflow{Items: []WorkflowItem{}}
	for _, raw := range list {
		item, _ := raw.(map[string]any)
		step, isStep := item["step"].(string)
		status, _ := item["status"].(string)
		if !isStep || !planStatuses[status] {
			return nil
		}
		w.Items = append(w.Items, WorkflowItem{Step: step, Status: status})
	}
	if explanation, ok := saved["explanation"].(string); ok {
		w.Explanation = explanation
	} else if explanation, ok := tool.Args["explanation"].(string); ok {
		w.Explanation = explanation
	}
	return w
}

// Plan is a response's plan: the entry it shows at (its first plan call,
// so updates keep one place) and its latest state.
type Plan struct {
	Anchor   string
	Workflow Workflow
}

// PlanOf is the response's plan, nil when it published none.
func PlanOf(r *Response) *Plan {
	var plan *Plan
	for _, e := range r.Entries {
		if e.Kind != "tool" {
			continue
		}
		if w := WorkflowFromTool(e.Tool); w != nil {
			if plan == nil {
				plan = &Plan{Anchor: e.Key}
			}
			plan.Workflow = *w
		}
	}
	return plan
}

var editingTool = regexp.MustCompile(`^(write|edit)$`)

// changedPaths are the files a finished, successful tool reports, or
// for write and edit calls without that report, their path argument.
func changedPaths(tool Tool) []string {
	if tool.Running || tool.IsError {
		return nil
	}
	if tool.Files != nil {
		paths := make([]string, len(tool.Files))
		for i, f := range tool.Files {
			paths[i] = f.Path
		}
		return paths
	}
	if path, ok := tool.Args["path"].(string); ok && editingTool.MatchString(tool.Name) {
		return []string{path}
	}
	return nil
}

// GeneratedPaths are the files the conversation's tools wrote, once
// each, for the transcript's file chips.
func GeneratedPaths(items []*Item) []string {
	seen := map[string]bool{}
	var paths []string
	for _, item := range items {
		if item.Kind != "response" {
			continue
		}
		for _, e := range item.Response.Entries {
			if e.Kind != "tool" {
				continue
			}
			for _, path := range changedPaths(e.Tool) {
				if !seen[path] {
					seen[path] = true
					paths = append(paths, path)
				}
			}
		}
	}
	return paths
}

// TurnPaths are the files one response's tools wrote.
func TurnPaths(r *Response) []string {
	var paths []string
	for _, e := range r.Entries {
		if e.Kind == "tool" {
			paths = append(paths, changedPaths(e.Tool)...)
		}
	}
	return paths
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+\d+(?:,\d+)? @@`)

var changingTool = regexp.MustCompile(`^(write|edit|apply_patch)$`)

// ChangedTurn is a turn whose tools changed files: its key, the user's
// words that started it, and a version that changes as its tools finish,
// to read its changes again.
type ChangedTurn struct {
	TurnID, Title, Version string
}

// ChangedTurns lists the turns with file changes, as the React window's
// changedTurns.
func ChangedTurns(items []*Item, messages []Message) []ChangedTurn {
	var turns []ChangedTurn
	for _, item := range items {
		if item.Kind != "response" {
			continue
		}
		changed := false
		var finished []string
		for _, e := range item.Response.Entries {
			if e.Kind != "tool" || e.Tool.Running {
				continue
			}
			finished = append(finished, e.Key)
			changed = changed || !e.Tool.IsError && (changingTool.MatchString(e.Tool.Name) || len(e.Tool.Files) > 0)
		}
		if !changed {
			continue
		}
		title := ""
		for _, m := range messages {
			if m.Role == "user" && first(m.TurnID, m.RequestID, m.ID) == item.Key {
				title = m.Text()
				break
			}
		}
		turns = append(turns, ChangedTurn{TurnID: item.Key, Title: title, Version: strings.Join(finished, ":")})
	}
	return turns
}

// PatchCounts counts a unified diff's added and removed lines, as the
// review's diff parser: lines after the first hunk header, headers (---,
// +++) and "\ No newline" markers excluded.
func PatchCounts(patch string) (additions, deletions int) {
	inHunk := false
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "), strings.HasPrefix(line, `\ No newline`):
		case hunkHeader.MatchString(line):
			inHunk = true
		case !inHunk:
		case strings.HasPrefix(line, "+"):
			additions++
		case strings.HasPrefix(line, "-"):
			deletions++
		}
	}
	return additions, deletions
}
