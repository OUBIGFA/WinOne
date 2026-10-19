package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestMCPExposurePreferencesRoundTripAndRejectUnknownModes(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"direct", "deferred", "codemode", "hidden"} {
		prefs := store.Preferences()
		raw := fmt.Sprintf(`[{"name":"fixture","enabled":true,"url":"http://127.0.0.1:12345","exposure":%q,"description":"Fixture service","toolExposure":{"private*":"hidden"}}]`, mode)
		if err = json.Unmarshal([]byte(raw), &prefs.MCPServers); err != nil {
			t.Fatal(err)
		}
		if _, err = store.SavePreferences(prefs); err != nil {
			t.Fatal(mode, err)
		}
		reopened, err := Open(root)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(reopened.Preferences().MCPServers)
		var servers []map[string]any
		if err = json.Unmarshal(encoded, &servers); err != nil {
			t.Fatal(err)
		}
		if servers[0]["exposure"] != mode || servers[0]["description"] != "Fixture service" {
			t.Fatal(string(encoded))
		}
	}
	prefs := store.Preferences()
	raw := `[{"name":"fixture","enabled":true,"url":"http://127.0.0.1:12345","exposure":"unknown"}]`
	json.Unmarshal([]byte(raw), &prefs.MCPServers)
	if _, err = store.SavePreferences(prefs); err == nil {
		t.Fatal("unknown exposure accepted")
	}
}

func TestWorkspacePreferenceOnlyAffectsNewConversations(t *testing.T) {
	root := t.TempDir()
	s, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.NewConversation("", "", "Old", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(root, "new-workspaces")
	if err = os.Mkdir(custom, 0700); err != nil {
		t.Fatal(err)
	}
	prefs := s.Preferences()
	prefs.WorkspaceDirectory = custom
	if _, err = s.SavePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	next, err := reopened.NewConversation("", "", "New", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(next.Directory) != custom {
		t.Fatalf("new conversation ignored preference: %s", next.Directory)
	}
	kept, _ := reopened.Conversation(old.ID)
	if kept.Directory != old.Directory {
		t.Fatal("preference relocated existing work")
	}
}

func TestInvalidPreferencesLeaveSavedConfigurationUntouched(t *testing.T) {
	s, _ := Open(t.TempDir())
	prefs := s.Preferences()
	prefs.SkillDirectories = []string{"relative/path"}
	if _, err := s.SavePreferences(prefs); err == nil {
		t.Fatal("relative skill path accepted")
	}
	if len(s.Preferences().SkillDirectories) != 0 {
		t.Fatal("invalid preferences persisted")
	}
}

func TestDefaultWorkspaceCanBeSavedBeforeTheFirstConversation(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(filepath.Join(root, "chat"))
	prefs := s.Preferences()
	if _, err := s.SavePreferences(prefs); err != nil {
		t.Fatal("first settings save requires a pre-existing conversation", err)
	}
}

func TestConversationDeletionRecyclesOnlyHistoryAndPreservesOnFailure(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(filepath.Join(root, "profile"))
	c, err := s.NewConversation("", root, "Delete", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "work.txt")
	if err = os.WriteFile(work, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDraft(c.ID, Draft{Text: "draft"}); err != nil {
		t.Fatal(err)
	}
	refused := errors.New("recycle unavailable")
	if err = s.DeleteConversation(c.ID, func(string) error { return refused }); !errors.Is(err, refused) {
		t.Fatal(err)
	}
	if _, err = s.Conversation(c.ID); err != nil {
		t.Fatal("failed recycling removed catalog entry")
	}
	var recycled string
	if err = s.DeleteConversation(c.ID, func(source string) error {
		recycled = source
		return os.Rename(source, filepath.Join(root, "recycled-history"))
	}); err != nil {
		t.Fatal(err)
	}
	if recycled != c.SessionDirectory {
		t.Fatal("wrong recycle target", recycled)
	}
	reopened, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.Snapshot().Conversations) != 0 {
		t.Fatal("deleted conversation returned after restart")
	}
	if raw, err := os.ReadFile(work); err != nil || string(raw) != "keep" {
		t.Fatal("work file affected", err)
	}
	if raw, err := os.ReadFile(filepath.Join(root, "recycled-history", "draft.json")); err != nil || len(raw) == 0 {
		t.Fatal("draft not recoverable", err)
	}
}

func TestDefaultThinkingLevelAndSessionDefaultsRoundTrip(t *testing.T) {
	root := t.TempDir()
	s, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	prefs := s.Preferences()
	prefs.DefaultThinkingLevel = "ultra"
	if _, err := s.SavePreferences(prefs); err == nil {
		t.Fatal("invalid thinking level accepted")
	}
	prefs.DefaultThinkingLevel = "high"
	disabled := false
	prefs.AutoRetry = &disabled
	prefs.AutoCompaction = &disabled
	if _, err := s.SavePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	reloaded := reopened.Preferences()
	if reloaded.DefaultThinkingLevel != "high" || reloaded.AutoRetry == nil || *reloaded.AutoRetry || reloaded.AutoCompaction == nil || *reloaded.AutoCompaction {
		t.Fatalf("session defaults must round-trip: %+v", reloaded)
	}
	conversation, err := reopened.NewConversation("", "", "New", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if conversation.ThinkingLevel != "high" {
		t.Fatalf("new conversation must inherit the default thinking level: %q", conversation.ThinkingLevel)
	}
}

func TestDurablePreferencesValidationAndRoundTrip(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "profile"))
	if err != nil {
		t.Fatal(err)
	}
	prefs := s.Preferences()
	if err = json.Unmarshal([]byte("{\"backgroundCompaction\":true,\"codeMode\":false,\"planning\":false,\"toolPreset\":\"read-only\",\"subagents\":false,\"maxSubagents\":2}"), &prefs); err != nil {
		t.Fatal(err)
	}
	saved, err := s.SavePreferences(prefs)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(saved)
	var data map[string]any
	_ = json.Unmarshal(raw, &data)
	for key, want := range map[string]any{"backgroundCompaction": true, "codeMode": false, "planning": false, "toolPreset": "read-only", "subagents": false, "maxSubagents": float64(2)} {
		if data[key] != want {
			t.Fatalf("%s did not round-trip: %s", key, raw)
		}
	}
	for _, sample := range []string{"{\"toolPreset\":\"invalid\"}", "{\"maxSubagents\":65}", "{\"maxSubagents\":-1}"} {
		invalid := s.Preferences()
		_ = json.Unmarshal([]byte(sample), &invalid)
		if _, err = s.SavePreferences(invalid); err == nil {
			t.Fatalf("invalid durable default accepted: %s", sample)
		}
	}
}
