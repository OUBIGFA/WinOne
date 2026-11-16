package configjson

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestMergeIndependentSettings(t *testing.T) {
	for _, tt := range []struct{ name, base, draft, current, want string }{
		{"nested", `{"a":1,"b":{"x":1,"y":2}}`, `{"a":2,"b":{"x":1,"y":2}}`, `{"a":1,"b":{"x":1,"y":3}}`, `{"a":2,"b":{"x":1,"y":3}}`},
		{"records", `{"p":[{"id":"a","x":1},{"id":"b","x":2}]}`, `{"p":[{"id":"b","x":2},{"id":"a","x":3}]}`, `{"p":[{"id":"a","x":1},{"id":"b","x":4},{"id":"c","x":5}]}`, `{"p":[{"id":"b","x":4},{"id":"a","x":3},{"id":"c","x":5}]}`},
		{"new-map", `{}`, `{"options":{"a":1}}`, `{"options":{"b":2}}`, `{"options":{"a":1,"b":2}}`},
		{"delete-and-edit", `{"p":[{"id":"a","x":1},{"id":"b","x":2}]}`, `{"p":[{"id":"a","x":1}]}`, `{"p":[{"id":"a","x":3},{"id":"b","x":2}]}`, `{"p":[{"id":"a","x":3}]}`},
		{"number-precision", `{"revision":9007199254740993,"x":0}`, `{"revision":9007199254740993,"x":1}`, `{"revision":9007199254740994,"x":0}`, `{"revision":9007199254740994,"x":1}`},
		{"identical", `{"x":1}`, `{"x":2}`, `{"x":2}`, `{"x":2}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Merge([]byte(tt.base), []byte(tt.draft), []byte(tt.current))
			if err != nil {
				t.Fatal(err)
			}
			var want any
			_ = json.Unmarshal([]byte(tt.want), &want)
			canonical, _ := json.Marshal(want)
			if tt.name == "number-precision" {
				if string(got) != tt.want {
					t.Fatalf("number changed: %s", got)
				}
			} else if !reflect.DeepEqual(got, canonical) {
				t.Fatalf("got %s want %s", got, canonical)
			}
		})
	}
}

func TestMergeRejectsOverlappingSettingsWithoutLeakingValues(t *testing.T) {
	for _, tt := range []struct{ base, draft, current string }{
		{`{"key":"original"}`, `{"key":"draft-secret"}`, `{"key":"remote-secret"}`},
		{`{"p":[{"id":"a","x":1}]}`, `{"p":[]}`, `{"p":[{"id":"a","x":2}]}`},
		{`{"a":["a"]}`, `{"a":["b"]}`, `{"a":["c"]}`},
		{`{"a":{"x":1}}`, `{"a":null}`, `{"a":{"x":2}}`},
	} {
		if _, err := Merge([]byte(tt.base), []byte(tt.draft), []byte(tt.current)); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("overlapping edits were accepted or values leaked", err)
		}
	}
}
