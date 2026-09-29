package host

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"winone/internal/settings"
)

func TestModuleRoutingKeepsRequestIdentity(t *testing.T) {
	store, _ := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	service := New(store)
	service.RegisterModule("wintray", func() Module { return Module{ID: "wintray", Name: "启动与托盘", State: "ready"} }, func(request Request) Response {
		if request.Method != "launch" {
			t.Errorf("module prefix leaked: %s", request.Method)
		}
		return Response{ID: request.ID, Result: json.RawMessage(`{"job":"1"}`)}
	})
	response := service.Handle(Request{Version: 1, ID: "original", Method: "wintray.launch"})
	if response.ID != "original" || response.Error != nil || string(response.Result) != `{"job":"1"}` {
		t.Fatalf("got %+v", response)
	}
}

func TestProtocolRejectsUnknownVersionAndMethod(t *testing.T) {
	s, _ := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	service := New(s)
	for _, request := range []Request{{Version: 99, ID: "1", Method: "snapshot"}, {Version: 1, ID: "2", Method: "execute.shell"}} {
		response := service.Handle(request)
		if response.Error == nil {
			t.Fatalf("accepted %+v", request)
		}
		if response.ID != request.ID {
			t.Fatal("lost correlation ID")
		}
	}
}

func TestServicePersistsAndReportsConflict(t *testing.T) {
	s, _ := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	service := New(s)
	value := s.Snapshot()
	value.Theme = "dark"
	data, _ := json.Marshal(value)
	r := Request{Version: 1, ID: "1", Method: "settings.save", Params: data}
	if response := service.Handle(r); response.Error != nil {
		t.Fatal(response.Error)
	}
	r.ID = "2"
	if response := service.Handle(r); response.Error == nil || response.Error.Code != "conflict" {
		t.Fatalf("got %+v", response)
	}
}
