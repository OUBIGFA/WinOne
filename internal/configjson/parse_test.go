package configjson

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseObjectPreservesNumbersAndRejectsAmbiguousDocuments(t *testing.T) {
	value, err := ParseObject([]byte(`{"number":9007199254740993,"items":[true,null,{"value":"text"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if value["number"] != json.Number("9007199254740993") {
		t.Fatal("large number lost precision")
	}
	items, ok := value["items"].([]any)
	if !ok || len(items) != 3 || items[0] != true || items[1] != nil || items[2].(map[string]any)["value"] != "text" {
		t.Fatal("nested values changed")
	}
	for _, raw := range []string{`[]`, `{} {}`, `{"name":1,"\u006eame":2}`, `{"items":[{"x":1,"x":2}]}`, `{"x":}`, "{\"invalid\":\"\xff\"}", strings.Repeat(`{"x":`, 34) + `0` + strings.Repeat(`}`, 34)} {
		if _, err := ParseObject([]byte(raw)); err == nil {
			t.Fatal("invalid document accepted", raw)
		}
		if err := ValidateObject([]byte(raw)); err == nil {
			t.Fatal("parser and validator disagreed", raw)
		}
	}
}
