package model

import (
	"encoding/json"
	"strings"
	"testing"
)

// These are the actual metadata fields produced by the bundled Pi display
// projection, not a UI mock or an invented media URL.
func TestStoredToolImageMetadataSurvivesNativeProjection(t *testing.T) {
	messages := decode[[]Message](t, `[{"role":"user","turnId":"one","content":"Read image"},{"role":"assistant","turnId":"one","content":[{"type":"toolCall","id":"read-image","name":"read","arguments":{"path":"input.png"}}]},{"role":"toolResult","turnId":"one","toolCallId":"read-image","content":[{"type":"text","text":"Image result"},{"type":"image","mimeType":"image/png","path":"C:/fixture/attachments/tool-media/owned.png"}]}]`)
	encoded, err := json.Marshal(messages[2].Content)
	if err != nil || !strings.Contains(string(encoded), "owned.png") {
		t.Fatalf("message media lost: %s %v", encoded, err)
	}
	items := Project(messages, nil, false, nil)
	tool := items[1].Response.Entries[0].Tool
	encoded, err = json.Marshal(tool)
	if err != nil || !strings.Contains(string(encoded), "owned.png") || !strings.Contains(string(encoded), "image/png") {
		t.Fatalf("tool media lost: %s %v", encoded, err)
	}
	if tool.Result != "Image result" {
		t.Fatalf("readable result changed: %#v", tool.Result)
	}
}

func TestLiveToolImageMetadataKeepsTheSameProjectionShape(t *testing.T) {
	state := FromSnapshot(Snapshot{Messages: decode[[]Message](t, `[{"role":"user","turnId":"one","content":"Read image"},{"role":"assistant","turnId":"one","content":[{"type":"toolCall","id":"read-image","name":"read","arguments":{"path":"input.png"}}]}]`)})
	state = Apply(state, event(t, `{"type":"pi_event","eventType":"tool_execution_end","toolCallId":"read-image","toolName":"read","turnId":"one","result":{"content":[{"type":"text","text":"Image result"},{"type":"image","mimeType":"image/png","path":"C:/fixture/attachments/tool-media/owned.png"}]}}`))
	items := Project(state.Messages, state.Tools, false, nil)
	encoded, err := json.Marshal(items[1].Response.Entries[0].Tool)
	if err != nil || !strings.Contains(string(encoded), "owned.png") {
		t.Fatalf("live media lost: %s %v", encoded, err)
	}
}
