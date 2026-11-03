package model

import (
	"reflect"
	"testing"
)

func TestToolsAreDescribedAsCindyNamesThem(t *testing.T) {
	cases := []struct {
		name  string
		input map[string]any
		kind  string
		param string
		file  bool
	}{
		{"read", map[string]any{"path": `E:\WinOne\build.log`}, "file", "build.log", true},
		{"write", map[string]any{"path": "docs/a.md", "content": "x"}, "file", "a.md", true},
		{"ls", map[string]any{"path": "src"}, "file", "src", false},
		{"bash", map[string]any{"command": "go test   ./..."}, "command", "go test   ./...", false},
		{"Bash", map[string]any{"command": "rm -rf x", "description": "Remove the build"}, "command", "Remove the build", false},
		{"grep", map[string]any{"pattern": "TODO", "path": "src"}, "search", "TODO", false},
		{"web_search", map[string]any{"query": "mygo"}, "web", "mygo", false},
		{"cindy_mcp_call_tool", map[string]any{"server": "docs", "tool": "make_pptx", "args": map[string]any{"outPath": "a.pptx"}}, "mcp", "make pptx", false},
		{"update_plan", map[string]any{"plan": []any{}}, "todo", "", false},
		{"custom", map[string]any{"query": "q"}, "generic", "q", false},
	}
	for _, c := range cases {
		d := DescribeTool(c.name, c.input)
		if d.Kind != c.kind || d.DisplayParam(c.input) != c.param || d.IsFileTool() != c.file {
			t.Errorf("%s: %+v param %q file %v", c.name, d, d.DisplayParam(c.input), d.IsFileTool())
		}
	}
	if d := DescribeTool("cindy_mcp_call_tool", map[string]any{"server": "docs", "tool": "make_pptx", "args": map[string]any{"outPath": "a.pptx"}}); d.CreatedPath != "a.pptx" || d.ToolName != "mcp:docs:make_pptx" {
		t.Errorf("gateway %+v", d)
	}
}

func TestFileSnapshotsKeepWhatTheToolSaw(t *testing.T) {
	read, ok := ToolFileSnapshot("read", nil, map[string]any{"content": []any{map[string]any{"type": "text", "text": "line"}}})
	if !ok || read != (FileSnapshot{Kind: "document", Source: "read", Text: "line"}) {
		t.Fatalf("read %+v", read)
	}
	edit, ok := ToolFileSnapshot("edit", map[string]any{"path": "a", "edits": []any{map[string]any{"oldText": "a\nb", "newText": "c"}}}, nil)
	if !ok || edit.Kind != "diff" || edit.Text != "@@ 1 @@\n-a\n-b\n+c" {
		t.Fatalf("edit %+v", edit)
	}
	if _, ok := ToolFileSnapshot("edit", map[string]any{"path": "a"}, nil); ok {
		t.Fatal("an edit without replacements has no snapshot")
	}
	// A JSON file's text is never read as an envelope.
	if text, _ := ToolResultText(`{"content":[{"type":"text","text":"x"}]}`); text != `{"content":[{"type":"text","text":"x"}]}` {
		t.Fatalf("text %q", text)
	}
}

func TestOnlyAFinishedPlanCallPublishesAPlan(t *testing.T) {
	args := map[string]any{"plan": []any{map[string]any{"step": "Read", "status": "completed"}, map[string]any{"step": "Fix", "status": "in_progress"}}}
	tool := Tool{ID: "p1", Name: "update_plan", Args: args, Result: "Plan updated"}
	w := WorkflowFromTool(tool)
	if w == nil || len(w.Items) != 2 || w.Items[1].Status != "in_progress" {
		t.Fatalf("workflow %+v", w)
	}
	saved := tool
	saved.Result = `{"items":[{"step":"Ship","status":"pending"}],"explanation":"Later"}`
	if w := WorkflowFromTool(saved); w == nil || w.Items[0].Step != "Ship" || w.Explanation != "Later" {
		t.Fatalf("saved %+v", w)
	}
	for _, broken := range []Tool{{Name: "update_plan", Args: args, Running: true, Result: "x"}, {Name: "update_plan", Args: args, IsError: true, Result: "x"},
		{Name: "update_plan", Args: args}, {Name: "update_plan", Args: map[string]any{"plan": []any{map[string]any{"step": "x", "status": "done"}}}, Result: "x"}} {
		if WorkflowFromTool(broken) != nil {
			t.Errorf("%+v published a plan", broken)
		}
	}
	later := tool
	later.ID, later.Args = "p2", map[string]any{"plan": []any{map[string]any{"step": "Read", "status": "completed"}}}
	plan := PlanOf(&Response{Entries: []Entry{{Kind: "tool", Key: "p1", Tool: tool}, {Kind: "tool", Key: "p2", Tool: later}}})
	if plan == nil || plan.Anchor != "p1" || len(plan.Workflow.Items) != 1 {
		t.Fatalf("plan %+v", plan)
	}
}

func TestTheConversationsFilesAndChangedTurns(t *testing.T) {
	messages := []Message{{ID: "u1", Role: "user", TurnID: "u1", Content: TextContent("Write the notes")}}
	response := &Response{Key: "u1", Entries: []Entry{
		{Kind: "tool", Key: "w1", Tool: Tool{Name: "write", Args: map[string]any{"path": "notes.md"}}},
		{Kind: "tool", Key: "w2", Tool: Tool{Name: "edit", Args: map[string]any{"path": "notes.md"}}},
		{Kind: "tool", Key: "b1", Tool: Tool{Name: "bash", Args: map[string]any{}, Files: []ChangedFile{{Path: "out.txt"}}}},
		{Kind: "tool", Key: "e1", Tool: Tool{Name: "write", Args: map[string]any{"path": "failed.md"}, IsError: true}},
		{Kind: "tool", Key: "r1", Tool: Tool{Name: "write", Args: map[string]any{"path": "late.md"}, Running: true}},
	}}
	items := []*Item{{Kind: "user", Key: "u1", Message: &messages[0]}, {Kind: "response", Key: "u1", Response: response}}
	if got := GeneratedPaths(items); !reflect.DeepEqual(got, []string{"notes.md", "out.txt"}) {
		t.Fatalf("generated %v", got)
	}
	turns := ChangedTurns(items, messages)
	if len(turns) != 1 || turns[0] != (ChangedTurn{TurnID: "u1", Title: "Write the notes", Version: "w1:w2:b1:e1"}) {
		t.Fatalf("turns %+v", turns)
	}
	add, del := PatchCounts("diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n-old\n+new\n+more\n context\n\\ No newline at end of file\n")
	if add != 2 || del != 1 {
		t.Fatalf("counts +%d -%d", add, del)
	}
}

func TestLinksResolveToTheFilesTheyName(t *testing.T) {
	files := []Attachment{{Name: "Report.docx", Path: `C:\uploads\Report.docx`}}
	cases := []struct {
		href string
		want LinkTarget
	}{
		{"src/main.go:12:3", LinkTarget{Kind: HrefText, Path: `E:\WinOne\src\main.go`, Line: 12, Column: 3}},
		{"docs/guide.md#L40", LinkTarget{Kind: HrefText, Path: `E:\WinOne\docs\guide.md`, Line: 40}},
		{"report.docx", LinkTarget{Kind: HrefText, Path: `C:\uploads\Report.docx`}},
		{`D:\shots\a.png`, LinkTarget{Kind: HrefImage, Path: `D:\shots\a.png`}},
		{"file:///C:/My%20Files/a.txt", LinkTarget{Kind: HrefText, Path: "C:/My Files/a.txt"}},
		{"src/", LinkTarget{Kind: HrefDirectory, Path: `E:\WinOne\src\`}},
		{"https://example.com/a.md", LinkTarget{Kind: HrefExternal, Path: `E:\WinOne\https:\\example.com\a.md`}},
		{"#usage", LinkTarget{Kind: HrefExternal, Path: `E:\WinOne\#usage`}},
	}
	for _, c := range cases {
		if got := TargetOf(c.href, `E:\WinOne\`, files); got != c.want {
			t.Errorf("%s: %+v", c.href, got)
		}
	}
	for text, want := range map[string]bool{"src/App.tsx": true, `C:\Users\me\a.txt`: true, "/etc/hosts.conf": true, "a.go:3": false,
		"package.json": false, "useState": false, "npm run build": false, "src/components/": true, "https://x.dev/a.js": false, "file:///C:/a.txt": true} {
		if _, ok := InlinePathCandidate(text, `E:\WinOne`, nil); ok != want {
			t.Errorf("%q candidate %v", text, ok)
		}
	}
}
