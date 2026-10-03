package captureprefs

import "testing"

func TestToolbarChoices(t *testing.T) {
	for _, tools := range [][]string{nil, {}, {"mosaic", "arrow"}} {
		if err := ValidateToolbar(tools); err != nil {
			t.Fatal(err)
		}
	}
	for _, tools := range [][]string{{"unknown"}, {"mosaic", "mosaic"}} {
		if ValidateToolbar(tools) == nil {
			t.Fatal("invalid toolbar accepted", tools)
		}
	}
}
