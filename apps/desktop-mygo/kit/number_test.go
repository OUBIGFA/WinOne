package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNumberFieldShowsExternalChangesBeforeDrawing(t *testing.T) {
	value := 1.0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			NumberField(c, &value, 0, 100, .1, "s", "Duration")
			if Button(c, "Preset", nil, false).Clicked() {
				value = 2.5
			}
		})
	}, 300, 200)
	if err := tt.Click("Preset"); err != nil {
		t.Fatal(err)
	}
	if field, ok := tt.Access("Duration"); !ok || field.Value != "2.5" {
		t.Fatalf("preset leaves the visible field stale: %+v", field)
	}
}

func TestNumberFieldKeepsTypingAndNormalizesOnBlur(t *testing.T) {
	value := 1.0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			NumberField(c, &value, 0, 100, .1, "s", "Duration")
			Button(c, "Done", nil, false)
		})
	}, 300, 200)
	if err := tt.Click("Duration"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("2.")
	if field, _ := tt.Access("Duration"); field.Value != "2." {
		t.Fatalf("partial decimal changed while typing: %+v", field)
	}
	tt.Type("5")
	if value != 2.5 {
		t.Fatalf("typed decimal = %v", value)
	}
	tt.Key(0, ui.KeyUp)
	if field, _ := tt.Access("Duration"); field.Value != "2.6" || value != 2.6 {
		t.Fatalf("arrow step left stale text: field=%+v value=%v", field, value)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("101")
	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	if field, _ := tt.Access("Duration"); field.Value != "2.6" || value != 2.6 {
		t.Fatalf("blur did not show the last valid value: field=%+v value=%v", field, value)
	}
}
