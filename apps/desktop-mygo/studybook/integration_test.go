package studybook

import (
	"context"
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"os"
	"path/filepath"
	"testing"
	"winone/desktopmygo/testhost"
	"winone/internal/ankiengine"
)

func ankiTestPath(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// The real native page and real Anki worker communicate through the same Host
// payloads. No HTTP listener, real account, window, or installed profile is used.
func TestNativeBookWithBundledEngine(t *testing.T) {
	python, library := os.Getenv("WINONE_TEST_ANKI_PYTHON"), os.Getenv("WINONE_TEST_ANKI_LIBRARY")
	if python == "" || library == "" {
		t.Skip("explicit offline integration")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	engine := ankiengine.New(ankiengine.Options{Python: python, Library: library, Worker: ankiTestPath("WINONE_TEST_ANKI_WORKER", filepath.Join(root, "modules/anki/worker.py")), Source: ankiTestPath("WINONE_TEST_ANKI_SOURCE", filepath.Join(root, "third_party/anki-connect/plugin")), Profile: t.TempDir()})
	defer engine.Close()
	call := func(method string, params, out any) error {
		_, err := engine.Call(context.Background(), method, params, out)
		return err
	}
	if err = call("legacy", map[string]any{"action": "createModel", "params": map[string]any{"modelName": "WinOne Word", "inOrderFields": []string{"Word", "Meaning"}, "cardTemplates": []map[string]string{{"Name": "Recall", "Front": "{{Word}}", "Back": "{{Meaning}}"}}}}, nil); err != nil {
		t.Fatal(err)
	}
	h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		var input struct {
			Method string
			Params json.RawMessage
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, err
		}
		var out json.RawMessage
		err := call(input.Method, input.Params, &out)
		return out, err
	})
	b := New(h.Env)
	defer b.Close()
	tt := ui.NewTester(b.View, 800, 1800)
	h.Settle(tt)
	for _, model := range b.state.NoteTypes {
		if model.Name == "WinOne Word" {
			b.modelID = model.ID
		}
	}
	b.tab = 1
	b.beginEdit(nil)
	h.Settle(tt)
	if b.edit == nil || b.model == nil || b.editorID == "" {
		t.Fatalf("native editor unavailable: %s", b.err)
	}
	b.edit.Fields["Word"] = "hello"
	b.edit.Fields["Meaning"] = "greeting"
	b.tags = "native"
	b.saveNote()
	h.Settle(tt)
	if b.err != "" || b.edit != nil || len(b.notes) != 1 || b.notes[0].Fields["Meaning"] != "greeting" {
		t.Fatalf("native add/save did not reach real collection: %s %+v", b.err, b.notes)
	}
	b.selected[b.notes[0].ID] = true
	var catalog toolCatalog
	if err = call("winone.tools", map[string]any{}, &catalog); err != nil {
		t.Fatal(err)
	}
	b.form.catalog = &catalog
	b.openTool("tags.add", map[string]any{"noteIds": []string{b.notes[0].ID}, "tags": []string{"reviewed"}})
	h.Settle(tt)
	if b.Dirty() {
		t.Fatal("rendering a prefilled form created unsaved changes")
	}
	if err = tt.Click(text("execute")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if b.err != "" {
		t.Fatal(b.err)
	}
	var ids []string
	if err = call("notes.search", map[string]any{"query": "tag:reviewed"}, nil); err != nil {
		t.Fatal(err)
	}
	var found browseResult
	if err = call("winone.browser", map[string]any{"query": "tag:reviewed"}, &found); err != nil {
		t.Fatal(err)
	}
	for _, n := range found.Notes {
		ids = append(ids, n.ID)
	}
	if len(ids) != 1 {
		t.Fatal("native management form failed to persist tags")
	}
	// Every upstream schema is rendered with production controls; optional
	// objects/arrays and enum alternatives must not panic or disappear.
	for _, action := range catalog.Actions {
		b.openTool(action.ID, map[string]any{})
		tt.Frame()
		if !tt.HasText(text("execute")) {
			t.Fatalf("management action has no execution control: %s", action.ID)
		}
	}
	b.openTool("decks.update", map[string]any{"deck": map[string]string{"name": "Default"}, "name": "Renamed"})
	tt.Frame()
	ref, ok := b.form.value["deck"].(map[string]any)
	if !ok || ref["name"] != "Default" || ref["id"] != nil {
		t.Fatalf("name reference changed while rendering: %#v", ref)
	}
	if err = tt.Click(text("execute")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if b.err != "" {
		t.Fatal(b.err)
	}
}
