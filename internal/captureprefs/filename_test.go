package captureprefs

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestFileNameContract(t *testing.T) {
	raw, err := os.ReadFile("../../tests/fixtures/capture-filenames.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Template string
		Sequence int
		Expected *string
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 9, 8, 7, 0, time.Local)
	for _, item := range cases {
		got, err := FileName(item.Template, at, item.Sequence)
		if item.Expected == nil {
			if err == nil {
				t.Errorf("accepted %q", item.Template)
			}
		} else if err != nil || got != *item.Expected {
			t.Errorf("%q: %q %v", item.Template, got, err)
		}
	}
}
