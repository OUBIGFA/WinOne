package learning

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnkiCreatesTypedCardEscapesHTMLAndFindsDuplicate(t *testing.T) {
	calls := []string{}
	exists := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Action string                     `json:"action"`
			Params map[string]json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Params == nil {
			t.Error("API 6 requires an object for params, even without arguments")
		}
		calls = append(calls, request.Action)
		var result any
		switch request.Action {
		case "version":
			result = 6
		case "deckNames":
			result = []string{"Study"}
		case "findNotes":
			if exists {
				result = []int{42}
			} else {
				result = []int{}
			}
		case "notesInfo":
			result = []any{map[string]any{"noteId": 42, "fields": map[string]any{"Word": map[string]string{"value": "&lt;word&gt;"}}}}
		case "createDeck":
			result = 1
		case "modelNames":
			result = []string{}
		case "createModel":
			var templates []map[string]string
			json.Unmarshal(request.Params["cardTemplates"], &templates)
			if len(templates) != 1 || !strings.Contains(templates[0]["Front"], "{{type:Word}}") {
				t.Error("typing card has no answer input")
			}
			result = map[string]any{"id": 1}
		case "addNote":
			var note struct{ Fields map[string]string }
			json.Unmarshal(request.Params["note"], &note)
			if note.Fields["Word"] != "&lt;word&gt;" || strings.Contains(note.Fields["Meaning"], "<script>") {
				t.Errorf("unescaped fields: %+v", note.Fields)
			}
			exists = true
			result = 42
		default:
			t.Errorf("unexpected action %s", request.Action)
		}
		json.NewEncoder(w).Encode(map[string]any{"result": result, "error": nil})
	}))
	defer server.Close()
	a := Anki{Endpoint: server.URL}
	note := Note{Word: "<word>", Meaning: "<script>example</script>", Example: "line1\nline2"}
	options := Defaults()
	options.Deck = "Study"
	options.CardMode = "typing"
	first, err := a.Add(context.Background(), options, note, nil)
	if err != nil || first.ID != 42 || first.Existing {
		t.Fatalf("%+v %v", first, err)
	}
	second, err := a.Add(context.Background(), options, note, nil)
	if err != nil || !second.Existing {
		t.Fatalf("%+v %v", second, err)
	}
	count := 0
	for _, call := range calls {
		if call == "addNote" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("duplicate addNote %d", count)
	}
}

func TestAnkiRejectsRemoteAndMalformedResponse(t *testing.T) {
	if _, err := (Anki{Endpoint: "https://example.com"}).Status(context.Background()); err == nil {
		t.Fatal("remote Anki accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"result":null}`)) }))
	defer server.Close()
	if _, err := (Anki{Endpoint: server.URL}).Status(context.Background()); err == nil {
		t.Fatal("malformed envelope accepted")
	}
}
