package aiservices

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
)

func TestPatternPreviewUsesRealUpstreamMatcherWithoutSaving(t *testing.T) {
	p, h, tt, _ := nativeAIWithRuntime(t)
	p.switchTab(tabRouting)
	settle(h, tt)
	r := p.routing
	r.begin(core.Preserved[routeGroup]{Value: routeGroup{ID: "not-saved", Name: "Preview", Members: []string{}, Match: []string{"fixture/*"}}})
	r.patternAPI.request(p, "POST", "/api/groups/preview", r.edit, func(raw json.RawMessage) {
		var value struct {
			Patterns []patternMatch `json:"patterns"`
		}
		if nativeDecode(&r.patternAPI, raw, &value) {
			r.previewPatterns = value.Patterns
		}
	})
	settle(h, tt)
	if r.patternAPI.err != "" || len(r.previewPatterns) != 1 || r.previewPatterns[0].Models != 1 {
		t.Fatalf("actual matching failed: %s %+v", r.patternAPI.err, r.previewPatterns)
	}
	r.load()
	settle(h, tt)
	for _, group := range r.state.Groups {
		if group.Value.ID == "not-saved" {
			t.Fatal("preview saved the group")
		}
	}
}

func TestMalformedMutationReceiptKeepsNativeDrafts(t *testing.T) {
	_, h, p, tt := setup(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
	defer server.Close()
	p.extensions.state.URL = server.URL + "/session/key/plugins.html"
	p.routing.begin(core.Preserved[routeGroup]{Value: routeGroup{ID: "draft", Name: "Saved"}})
	p.routing.edit.Value.Name = "Unsaved"
	p.routing.saveGroup()
	settle(h, tt)
	if !p.routing.dirty() || p.routing.api.err == "" {
		t.Fatal("invalid group receipt discarded the draft")
	}
	p.plugins.optionsSpec, p.plugins.optionsSaved, p.plugins.optionsText = "fixture", `{}`, `{"kept":true}`
	p.plugins.act("options", map[string]any{"spec": "fixture", "options": map[string]any{"kept": true}})
	settle(h, tt)
	if !p.plugins.dirty() || p.plugins.api.err == "" {
		t.Fatal("invalid plugin receipt discarded the draft")
	}
}
func TestOpeningAWeightedGroupDoesNotDirtyItsDefaultWeights(t *testing.T) {
	_, h, p, _ := setup(t)
	r := p.routing
	r.state = &routeGroups{}
	r.begin(core.Preserved[routeGroup]{Value: routeGroup{ID: "weighted", Name: "Weighted", Routing: "weight", Members: []string{"fixture/model"}}})
	r.editorTab = 1
	tt := ui.NewTester(r.editor, 900, 4000)
	settle(h, tt)
	if r.dirty() || r.edit.Value.Weights != nil {
		t.Fatal("merely viewing a group wrote implicit weights")
	}
}

func TestNetworkErrorDoesNotExposeRuntimeCapability(t *testing.T) {
	_, h, p, tt := setup(t)
	p.extensions.state.URL = "http://127.0.0.1:9/session/private-capability/plugins.html"
	var request nativeAPI
	request.request(p, "GET", "/api/groups", nil, func(json.RawMessage) { t.Fatal("unreachable request succeeded") })
	settle(h, tt)
	if request.err == "" || strings.Contains(request.err, "private-capability") {
		t.Fatalf("network error exposed capability: %q", request.err)
	}
}
