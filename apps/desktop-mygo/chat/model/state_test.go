package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The cases below port chat-state.test.ts and chat-process.test.ts of the
// React window, so both windows read the Host's stream the same way.

func decode[T any](t *testing.T, text string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatalf("decode %s: %v", text, err)
	}
	return value
}

func event(t *testing.T, text string) Event { return decode[Event](t, text) }

func texts(messages []Message) []string {
	var out []string
	for _, m := range messages {
		out = append(out, m.Text())
	}
	return out
}

func kinds(items []*Item) []string {
	var out []string
	for _, item := range items {
		out = append(out, item.Kind)
	}
	return out
}

func TestRetryKeepsTheUserBeforeTheFirstReplyAndRetiresTheOldBranch(t *testing.T) {
	state := FromSnapshot(Snapshot{Messages: decode[[]Message](t, `[{"id":"old-user","role":"user","content":"Retry me"},{"id":"old-answer","role":"assistant","content":"Retired answer"}]`)})
	state = Apply(state, event(t, `{"type":"branch_reset","state":{"messages":[{"id":"retry","requestId":"retry","turnId":"retry","role":"user","content":"Retry me","pending":true}],"hasOlder":false,"streaming":true}}`))
	state = Apply(state, event(t, `{"type":"delta","kind":"text_delta","turnId":"retry","timestamp":10,"text":"New answer"}`))
	for _, m := range state.Messages {
		if m.Text() == "Retired answer" {
			t.Fatal("the retired branch is still shown")
		}
	}
	items := Project(state.Messages, state.Tools, state.Running, state.Operations)
	if got := kinds(items); !reflect.DeepEqual(got, []string{"user", "response"}) || items[0].Key != "retry" {
		t.Fatalf("items %v key %q", got, items[0].Key)
	}
}

func TestReceiptIdentityWhenASteerOvertakesQueuedInput(t *testing.T) {
	state := FromSnapshot(Snapshot{Messages: decode[[]Message](t, `[{"role":"user","requestId":"queue","content":"Next task","pending":true},{"role":"user","requestId":"steer","content":"Correction","pending":true}]`)})
	state = Apply(state, event(t, `{"type":"pi_event","eventType":"message_start","message":{"id":"saved-steer","requestId":"steer","turnId":"steer","role":"user","content":"Correction","timestamp":10}}`))
	state = Apply(state, event(t, `{"type":"pi_event","eventType":"message_end","message":{"id":"answer","turnId":"steer","role":"assistant","content":"Corrected","timestamp":20}}`))
	if got := texts(state.Messages); !reflect.DeepEqual(got, []string{"Correction", "Corrected", "Next task"}) || state.Messages[0].Pending {
		t.Fatalf("messages %v", got)
	}
}

func TestALiveResponseAnchorsAfterItsUser(t *testing.T) {
	items := Project(decode[[]Message](t, `[{"role":"assistant","turnId":"retry","timestamp":10,"content":"Streaming"},{"role":"user","turnId":"retry","content":"Retry me"}]`), nil, true, nil)
	if got := kinds(items); !reflect.DeepEqual(got, []string{"user", "response"}) || items[0].Key != "retry" {
		t.Fatalf("items %v", got)
	}
}

func TestASnapshotIDReconcilesWithTheLiveAssistant(t *testing.T) {
	state := FromSnapshot(Snapshot{Messages: decode[[]Message](t, `[{"id":"saved-answer","role":"assistant","turnId":"turn","timestamp":10,"content":"Partial"}]`)})
	state = Apply(state, event(t, `{"type":"pi_event","eventType":"message_end","message":{"role":"assistant","turnId":"turn","timestamp":10,"content":"Full"}}`))
	if len(state.Messages) != 1 || state.Messages[0].Text() != "Full" || state.Messages[0].ID != "saved-answer" {
		t.Fatalf("messages %+v", state.Messages)
	}
}

func TestPausedTaskOwnershipAndResume(t *testing.T) {
	state := Apply(Empty(), event(t, `{"type":"durable_state","paused":true,"taskGraph":{"scheduling":"paused","total":1,"tasks":[{"id":3,"kind":"fixture","conversationId":1,"background":false,"state":"ready"}]}}`))
	if state.Paused == nil || !*state.Paused || state.TaskGraph == nil || state.TaskGraph.Scheduling != "paused" {
		t.Fatalf("paused %+v", state)
	}
	state = Apply(state, event(t, `{"type":"resumed","state":{"messages":[{"role":"user","content":"Keep history"}],"hasOlder":false,"paused":false,"streaming":true,"taskGraph":{"scheduling":"running","total":1,"tasks":[]}}}`))
	if state.Paused == nil || *state.Paused || !state.Running || state.Messages[0].Text() != "Keep history" {
		t.Fatalf("resumed %+v", state)
	}
}

func TestWorkflowUpdatesStayInTheSnapshot(t *testing.T) {
	state := Apply(Empty(), event(t, `{"type":"durable_state","workflow":{"items":[{"step":"Read configuration","status":"in_progress"}],"explanation":"Check before changing"}}`))
	if state.Workflow == nil || state.Workflow.Explanation != "Check before changing" {
		t.Fatalf("workflow %+v", state.Workflow)
	}
	state = Apply(state, event(t, `{"type":"durable_state","workflow":{"items":[{"step":"Read configuration","status":"completed"}]}}`))
	if state.Workflow.Items[0].Status != "completed" {
		t.Fatalf("workflow %+v", state.Workflow)
	}
}

func TestToolActivityAndDeltasThenSavedMessages(t *testing.T) {
	state := Empty()
	state = Apply(state, event(t, `{"type":"pi_event","eventType":"message_start","message":{"role":"assistant","timestamp":1,"content":[]}}`))
	state = Apply(state, event(t, `{"type":"delta","kind":"thinking_delta","index":0,"text":"Consider"}`))
	state = Apply(state, event(t, `{"type":"delta","kind":"text_delta","index":1,"text":"Answer"}`))
	want := []Part{{Type: "thinking", Thinking: "Consider"}, {Type: "text", Text: "Answer"}}
	if got := state.Messages[0].Content.Parts; !reflect.DeepEqual(got, want) {
		t.Fatalf("parts %+v", got)
	}
	state = Apply(state, event(t, `{"type":"pi_event","eventType":"tool_execution_start","toolCallId":"write1","toolName":"write","args":{"path":"proof.txt"}}`))
	if !state.Tools[0].Running {
		t.Fatal("the tool is not running")
	}
	state = Apply(state, event(t, `{"type":"settled","status":"completed","state":{"messages":[{"id":"saved1","role":"assistant","content":[{"type":"text","text":"Saved answer"}]}],"hasOlder":false}}`))
	if len(state.Messages) != 1 || state.Messages[0].ID != "saved1" || state.Running {
		t.Fatalf("settled %+v", state)
	}
}

func TestAPendingPermissionStaysUntilResolved(t *testing.T) {
	state := Apply(Empty(), event(t, `{"type":"approval","request":{"id":"approve1","tool":"powershell","args":{"command":"build"}}}`))
	if state.Approvals[0].ID != "approve1" {
		t.Fatal("no approval")
	}
	state = Apply(state, event(t, `{"type":"approval_cancelled","id":"approve1"}`))
	if len(state.Approvals) != 0 {
		t.Fatal("the approval stayed")
	}
}

func TestTheOptimisticUserRowReconciles(t *testing.T) {
	state := FromSnapshot(Snapshot{Messages: decode[[]Message](t, `[{"id":"request-1","requestId":"request-1","role":"user","content":"Same text","timestamp":10}]`)})
	state = Apply(state, event(t, `{"type":"pi_event","eventType":"message_start","message":{"requestId":"request-1","turnId":"request-1","role":"user","content":"Same text","timestamp":20}}`))
	if len(state.Messages) != 1 {
		t.Fatalf("live start duplicated: %d", len(state.Messages))
	}
	state = Apply(state, event(t, `{"type":"pi_event","eventType":"message_end","message":{"id":"saved-1","requestId":"request-1","role":"user","content":"Same text","timestamp":20}}`))
	if len(state.Messages) != 1 || state.Messages[0].ID != "saved-1" {
		t.Fatalf("saved %+v", state.Messages)
	}
	state = Apply(state, event(t, `{"type":"pi_event","eventType":"message_start","message":{"requestId":"request-2","role":"user","content":"Same text","timestamp":30}}`))
	if len(state.Messages) != 2 {
		t.Fatalf("a new request merged: %d", len(state.Messages))
	}
}

func TestUsageAndCollaborationLeaveMessages(t *testing.T) {
	state := Apply(Empty(), event(t, `{"type":"usage","usage":{"input":10,"output":5,"cacheRead":0,"cacheWrite":0,"totalTokens":15,"cost":0,"costKnown":false},"contextUsage":{"tokens":15,"contextWindow":1000,"percent":1.5}}`))
	if *state.ContextUsage.Percent != 1.5 || state.Usage.TotalTokens != 15 {
		t.Fatalf("usage %+v", state)
	}
	state = Apply(state, event(t, `{"type":"durable_state","subagents":[{"name":"scout","status":"running"}],"backgroundRunning":true}`))
	if state.Subagents[0].Name != "scout" || !state.BackgroundRunning || len(state.Messages) != 0 {
		t.Fatalf("collaboration %+v", state)
	}
}

func TestCompletionInputsStayOutOfUserBubbles(t *testing.T) {
	items := Project(decode[[]Message](t, `[{"id":"owner","role":"user","content":"Do the task"},{"id":"wake","turnId":"wake","role":"user","hiddenInput":true,"content":"Internal task completion"},{"id":"result","turnId":"wake","role":"assistant","content":"Task result"}]`), nil, false, nil)
	var users []string
	var wake *Response
	for _, item := range items {
		if item.Kind == "user" {
			users = append(users, item.Message.ID)
		}
		if item.Kind == "response" && item.Key == "wake" {
			wake = item.Response
		}
	}
	if !reflect.DeepEqual(users, []string{"owner"}) || wake == nil || wake.Answer == nil || wake.Answer.Text() != "Task result" {
		t.Fatalf("users %v wake %+v", users, wake)
	}
}

func TestThinkingWorkAndToolsGroupWithTheAnswerOutside(t *testing.T) {
	items := Project(decode[[]Message](t, `[
		{"id":"user","role":"user","content":"Create a file","timestamp":1},
		{"id":"work","role":"assistant","timestamp":10,"content":[{"type":"thinking","thinking":"Check the request"},{"type":"text","text":"I will create it"},{"type":"toolCall","id":"write","name":"write","arguments":{"path":"file.txt","content":"test"}}]},
		{"id":"result","role":"toolResult","toolCallId":"write","toolName":"write","timestamp":20,"content":[{"type":"text","text":"File saved"}]},
		{"id":"answer","role":"assistant","timestamp":30,"durationMs":5,"content":[{"type":"text","text":"Created file.txt"}]}]`), nil, false, nil)
	if len(items) != 2 || items[1].Kind != "response" {
		t.Fatalf("items %v", kinds(items))
	}
	r := items[1].Response
	var entryKinds []string
	for _, e := range r.Entries {
		entryKinds = append(entryKinds, e.Kind)
	}
	if r.Answer == nil || r.Answer.ID != "answer" || !reflect.DeepEqual(entryKinds, []string{"thinking", "commentary", "tool"}) {
		t.Fatalf("answer %+v entries %v", r.Answer, entryKinds)
	}
	tool := r.Entries[2].Tool
	if tool.Result != "File saved" || tool.Running || r.DurationMs == nil || *r.DurationMs != 25 {
		t.Fatalf("tool %+v duration %v", tool, r.DurationMs)
	}
}

func TestLiveToolEventsDeduplicateAndTurnsSeparate(t *testing.T) {
	items := Project(decode[[]Message](t, `[{"id":"u1","role":"user","content":"One"},{"role":"assistant","content":[{"type":"toolCall","id":"read","name":"read","arguments":{"path":"a"}}]},{"id":"u2","role":"user","content":"Two"},{"role":"assistant","content":[{"type":"text","text":"Second"}]}]`),
		[]Tool{{ID: "read", Name: "read", Args: map[string]any{"path": "a"}, Result: "Read"}}, false, nil)
	responses, tools := 0, 0
	for _, item := range items {
		if item.Kind == "response" {
			responses++
			for _, e := range item.Response.Entries {
				if e.Kind == "tool" {
					tools++
				}
			}
		}
	}
	if responses != 2 || tools != 1 {
		t.Fatalf("responses %d tools %d", responses, tools)
	}
}

func TestNewToolActivityWaitsForItsOwnTurn(t *testing.T) {
	items := Project(decode[[]Message](t, `[{"id":"u1","role":"user","content":"One"},{"id":"a1","role":"assistant","content":"Done"},{"id":"u2","role":"user","content":"Two"}]`),
		[]Tool{{ID: "new-tool", Name: "read", Args: map[string]any{}, Running: true}}, true, nil)
	var firstReply *Response
	for _, item := range items {
		if item.Kind == "response" && item.Key == "u1" {
			firstReply = item.Response
		}
	}
	if firstReply == nil || firstReply.Running || len(firstReply.Entries) != 0 {
		t.Fatalf("first %+v", firstReply)
	}
	last := items[len(items)-1]
	if last.Kind != "response" || last.Key != "u2" || last.Response.Entries[0].Kind != "tool" {
		t.Fatalf("last %+v", last)
	}
}

func TestContentKeepsItsShape(t *testing.T) {
	for _, text := range []string{`"plain"`, `[{"type":"text","text":"a"}]`, `[]`} {
		var c Content
		if err := json.Unmarshal([]byte(text), &c); err != nil {
			t.Fatal(err)
		}
		out, _ := json.Marshal(c)
		if string(out) != text {
			t.Fatalf("%s became %s", text, out)
		}
	}
}
