package core

import (
	"encoding/json"
	"testing"
)

type program struct {
	Name string `json:"name"`
	Args string `json:"args,omitempty"`
}

func TestPreservedKeepsUnknownFieldsAndClearsOmittedOnes(t *testing.T) {
	var doc Preserved[program]
	if err := json.Unmarshal([]byte(`{"name":"a","args":"-x","future":{"k":1},"other":true}`), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Value.Name != "a" || doc.Value.Args != "-x" {
		t.Fatalf("decoded %+v", doc.Value)
	}
	doc.Value.Name, doc.Value.Args = "b", ""
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out["name"] != "b" || out["other"] != true || out["future"] == nil {
		t.Fatalf("lost fields: %s", data)
	}
	if _, ok := out["args"]; ok {
		t.Fatalf("cleared field kept its old value: %s", data)
	}
}

func TestPreservedInSliceAndNull(t *testing.T) {
	var list []Preserved[program]
	if err := json.Unmarshal([]byte(`[{"name":"a","x":1},{"name":"b"}]`), &list); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(list)
	if string(data) != `[{"name":"a","x":1},{"name":"b"}]` {
		t.Fatalf("round trip %s", data)
	}
	var empty Preserved[program]
	if err := json.Unmarshal([]byte(`null`), &empty); err != nil {
		t.Fatal(err)
	}
}

func TestActivityAggregatesAndDiscards(t *testing.T) {
	var a Activity
	reverted := false
	a.Report("one", true, false, func() { reverted = true })
	a.Report("two", false, true, nil)
	if dirty, busy := a.State(); !dirty || !busy {
		t.Fatalf("state %v %v", dirty, busy)
	}
	a.Discard()
	if !reverted {
		t.Fatal("dirty section not reverted")
	}
	a.Report("one", false, false, nil)
	a.Report("two", false, false, nil)
	if dirty, busy := a.State(); dirty || busy {
		t.Fatal("clean reports kept activity")
	}
}
