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

func TestRoutingTitlesKeepParentsNativeSessionsAndKnownNames(t *testing.T) {
	_, h, p, tt := setup(t)
	// Drive the async read without the hidden routing page canceling it.
	tt = ui.NewTester(func(c *ui.Context) {}, 560, 900)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			IDs      []string
			Sessions []string
			Groups   bool
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Groups {
			w.Write([]byte(`{"names":{"parent":"Parent title"},"parents":{"2":"parent"},"matched":{"2":true}}`))
		} else {
			if len(request.Sessions) != 1 || request.Sessions[0] != "claude:native" {
				t.Errorf("native session lost: %+v", request)
			}
			w.Write([]byte(`{"titles":{"claude:native":"","claude:other":"Other title"}}`))
		}
	}))
	defer server.Close()
	p.extensions.state.URL = server.URL + "/session/key/plugins.html"
	trace := &p.routing.trace
	trace.groupBy = "session"
	trace.titles = map[string]string{"claude:native": "Known title"}
	trace.routes = []traceRoute{{ID: 1, Agent: "claude", Session: "proxy", NativeSession: "native"}, {ID: 2, Agent: "codex", Session: "child", SessionTitle: "Child title"}}
	trace.loadTitles(p, trace.routes, false)
	settle(h, tt)
	if trace.groupLabel(trace.routes[0]) != "Known title" || trace.groupLabel(trace.routes[1]) != "Parent title" || trace.groupKey(trace.routes[1]) != "codex · parent" {
		t.Fatalf("session names/groups differ from upstream: %+v %+v", trace.routes, trace.titles)
	}
}

func TestRoutingEvidenceKeepsPercentageUnitsAndScalarLists(t *testing.T) {
	_, _, p, _ := setup(t)
	var row traceRoute
	json.Unmarshal([]byte(`{"id":1,"done":true,"order":[{"id":"one","name":"One","known":true,"used":71}]}`), &row)
	tt := ui.NewTester(func(c *ui.Context) {
		p.routing.trace.evidenceView(c, p, &row)
		traceFields(c, json.RawMessage(`42`), func(s string) string { return s })
	}, 560, 900)
	tt.Frame()
	if !tt.HasText(nativeT("quotaUsed")+" 71.0%") || !tt.HasText("42") {
		t.Fatalf("evidence units/scalars lost: %q", tt.Texts())
	}
	if traceStatus(traceRoute{Done: true, Fail: "quota"}) != "failed" {
		t.Fatal("failed request shown as successful")
	}
}

// The real history handler returns dated request counts, not string dates.
func TestRoutingHistoryAcceptsUpstreamDayIndex(t *testing.T) {
	_, h, p, tt := setup(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"days":[{"day":"2026-11-06","requests":3}],"routes":[],"cut":true}`))
	}))
	defer server.Close()
	p.extensions.state.URL = server.URL + "/session/key/plugins.html"
	trace := &p.routing.trace
	trace.day = "2026-11-06"
	trace.load(p)
	settle(h, tt)
	if trace.api.err != "" || !trace.loaded {
		t.Fatalf("upstream history was rejected: %s", trace.api.err)
	}
}

func TestRoutingTemplateSavesAsNewGroup(t *testing.T) {
	_, h, p, tt := setup(t)
	var from string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&got)
		json.Unmarshal(got["from"], &from)
		w.Write([]byte(`{"groups":[],"models":[],"pools":[]}`))
	}))
	defer server.Close()
	p.extensions.state.URL = server.URL + "/session/key/plugins.html"
	r := p.routing
	r.beginNew(core.Preserved[routeGroup]{Value: routeGroup{ID: "template", Name: "Template", Members: []string{"vendor/model"}}})
	if !r.dirty() {
		t.Fatal("a new template must be immediately saveable")
	}
	r.edit.Value.ID = "mine"
	r.saveGroup()
	settle(h, tt)
	if from != "" {
		t.Fatalf("new template must not rename an existing group: from=%q", from)
	}
}

func TestRoutingSelectAllOnlySelectsVisibleGroups(t *testing.T) {
	r := &routingPanel{search: "daily", selected: map[string]bool{}, state: &routeGroups{}}
	for _, g := range []routeGroup{{ID: "a", Name: "Daily"}, {ID: "b", Name: "Other"}, {ID: "c", Name: "Daily", Hidden: true}} {
		r.state.Groups = append(r.state.Groups, core.Preserved[routeGroup]{Value: g})
	}
	r.selectVisible()
	if len(r.selected) != 1 || !r.selected["a"] {
		t.Fatalf("selected invisible groups: %v", r.selected)
	}
}

func TestRoutingHistoryOpensTheFullRoute(t *testing.T) {
	_, h, p, _ := setup(t)
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/gateway/route") {
			reads++
			if r.URL.Query().Get("id") != "7" || r.URL.Query().Get("day") != "2026-11-06" {
				t.Errorf("route lookup lost identity: %s", r.URL.RequestURI())
			}
			w.Write([]byte(`{"id":7,"time":"2026-11-06T12:00:00Z","model":"group/daily","done":true,"order":[{"id":"account-one","name":"Primary provider","who":"person@example.com","model":"vendor/model"}],"tries":[]}`))
			return
		}
		w.Write([]byte(`{"days":[],"routes":[]}`))
	}))
	defer server.Close()
	p.extensions.state.URL = server.URL + "/session/key/plugins.html"
	trace := &p.routing.trace
	trace.loaded, trace.paused, trace.day = true, true, "2026-11-06"
	var row traceRoute
	json.Unmarshal([]byte(`{"id":7,"time":"2026-11-06T12:00:00Z","model":"group/daily","done":true}`), &row)
	trace.routes = []traceRoute{row}
	tt := ui.NewTester(func(c *ui.Context) { trace.View(c, p) }, 1000, 1600)
	settle(h, tt)
	if err := tt.Click(nativeT("request") + " 7"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if reads != 1 {
		t.Fatalf("history detail must load complete upstream route; reads=%d", reads)
	}
	if _, ok := tt.Find("1. Primary provider · person@example.com · vendor/model"); !ok {
		t.Fatal("complete candidate evidence not displayed")
	}
}

func TestRoutingEvidenceRespectsAccountHiding(t *testing.T) {
	_, h, p, _ := setup(t)
	p.draft.Value.settings().HideAccounts = true
	var row traceRoute
	json.Unmarshal([]byte(`{"id":7,"order":[{"id":"provider/person@example.com","who":"person@example.com","name":"Provider","model":"model"}],"tries":[{"id":"provider/person@example.com","model":"model","status":200}],"group":{"account":"person@example.com"}}`), &row)
	p.routing.trace.routes = []traceRoute{row}
	tt := ui.NewTester(func(c *ui.Context) { p.routing.trace.evidenceView(c, p, &row) }, 1000, 1600)
	settle(h, tt)
	for _, text := range tt.Texts() {
		if strings.Contains(text, "person@example.com") {
			t.Fatalf("hidden account exposed in route evidence: %q", text)
		}
	}
}

func TestRoutingGroupsDoNotMergeKeysWithTheSameName(t *testing.T) {
	trace := nativeTrace{byKey: true}
	a := traceRoute{CallerKey: "key-1", CallerKeyName: "Team"}
	b := traceRoute{CallerKey: "key-2", CallerKeyName: "Team"}
	if trace.groupKey(a) == trace.groupKey(b) {
		t.Fatal("different caller keys were merged by display name")
	}
}

func TestRoutingLiveUpdatesMergeChangesAndDropExpiredRequests(t *testing.T) {
	_, h, p, tt := setup(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("after") != "4" || r.URL.Query().Get("wait") != "1" {
			t.Errorf("not reading incremental live updates: %s", r.URL.RequestURI())
		}
		w.Write([]byte(`{"seq":5,"ids":[2,3],"routes":[{"id":3,"model":"new","done":true,"status":200}]}`))
	}))
	defer server.Close()
	p.extensions.state.URL = server.URL + "/session/key/plugins.html"
	tr := &p.routing.trace
	tr.routes = []traceRoute{{ID: 1, Model: "expired"}, {ID: 2, Model: "kept"}}
	tr.loaded, tr.seq = true, 4
	tr.load(p)
	settle(h, tt)
	if len(tr.routes) != 2 || tr.routes[0].ID != 2 || tr.routes[1].ID != 3 || tr.seq != 5 {
		t.Fatalf("incremental trace was not merged: %+v", tr.routes)
	}
}

func TestRoutingRecordLinkSelectsPrivacyHistory(t *testing.T) {
	_, h, p, _ := setup(t)
	p.maskRecord = "fixture-record"
	a := newAdvancedPanel(p)
	tt := ui.NewTester(func(c *ui.Context) { a.privacy(c, &magpieCatalog{}, false) }, 1000, 1800)
	settle(h, tt)
	if a.privacyTab != 2 || a.records == nil {
		t.Fatal("record link opened protection instead of history")
	}
	a.Close()
}
