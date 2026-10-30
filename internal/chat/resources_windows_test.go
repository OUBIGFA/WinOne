//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/ai"
)

func TestResourcesBeforeOpenRemainAvailableWithoutAModel(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit Pi runtime fixture")
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	os.Mkdir(workspace, 0700)
	agents := filepath.Join(workspace, "AGENTS.md")
	os.WriteFile(agents, []byte("Resource inspection fixture"), 0600)
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation("", workspace, "Resources", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	service := NewService(ctx, store, ServiceOptions{Bun: bun, Worker: worker, Configure: func(Conversation) (map[string]any, func(), error) { return nil, nil, errors.New("fixture: no model") }})
	defer func() {
		ending, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := service.Stop(ending); err != nil {
			t.Error(err)
		}
	}()
	// No preceding Open call: this is the sidebar-before-transcript race.
	raw, err := service.Resources(ctx, c.ID, 77)
	if err != nil {
		t.Fatal(err)
	}
	var resources struct {
		Agents []struct {
			Path string `json:"path"`
		} `json:"agents"`
	}
	if err = json.Unmarshal(raw, &resources); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range resources.Agents {
		found = found || item.Path == agents
	}
	if !found {
		t.Fatal("local resources were hidden by model setup", string(raw))
	}
	again, err := service.Resources(ctx, c.ID, 77)
	if err != nil || len(again) == 0 {
		t.Fatal("repeat resource read failed", err)
	}
	service.Release(c.ID, 77)
}

func TestModelFreeResourceReadPreservesCompanionSelections(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit Pi runtime fixture")
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	skill := filepath.Join(workspace, ".pi", "skills", "ambient")
	if err := os.MkdirAll(skill, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: ambient\ndescription: Must remain unselected\n---\nDo not load implicitly."), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", workspace, "Companion resources", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	service := NewService(ctx, store, ServiceOptions{Bun: bun, Worker: worker,
		Configure: func(Conversation) (map[string]any, func(), error) {
			return nil, nil, errors.New("fixture: model removed")
		},
		ResourceConfig: func(_ context.Context, c Conversation) (map[string]any, error) {
			return map[string]any{"cwd": c.Directory, "agentDir": filepath.Join(root, "agent"), "onebot": map[string]any{"capabilities": map[string]any{"skillMode": "allowlist", "skills": []string{}}, "skillDirectories": []string{}}}, nil
		},
	})
	defer service.Stop(context.Background())
	raw, err := service.Resources(ctx, conversation.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Skills []json.RawMessage `json:"skills"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 0 {
		t.Fatalf("model-free fallback broadened companion resources: %s", raw)
	}
}
