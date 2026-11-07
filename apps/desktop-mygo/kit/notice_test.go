package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
)

func TestNoticesDismissOnlyTheirCurrentContentAndNewMessagesReturn(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for _, tone := range []Tone{Info, Failure, Caution} {
			message := "Current warning"
			invalid := true
			actions := 0
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(Theme(c.Preferences(), false))
				ui.Column(c).Fill().Children(func() {
					Notice(c, tone, "Notice", message, nil)
					if ui.Button(c, "Protected action").Disabled(invalid).Clicked() {
						actions++
					}
				})
			}, 560, 400)
			if err := tt.Click(i18n.T("notices.dismiss")); err != nil {
				t.Fatal(err)
			}
			for range 5 {
				tt.Frame()
			}
			if tt.HasText(message) || !invalid {
				t.Fatal("dismiss reappeared or removed validation")
			}
			_ = tt.Click("Protected action")
			tt.Frame()
			if actions != 0 {
				t.Fatal("closing a warning enabled its protected action")
			}
			message = "New warning"
			tt.Frame()
			if !tt.HasText(message) {
				t.Fatal("new content stayed dismissed")
			}
		}
	}
}
func TestInlineErrorKeyboardCloseDoesNotClearTheUnderlyingFailure(t *testing.T) {
	failure := "Invalid form value"
	saves := 0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			InlineError(c, failure)
			if ui.Button(c, "Save").Disabled(failure != "").Clicked() {
				saves++
			}
		})
	}, 560, 250)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	for range 3 {
		tt.Frame()
	}
	if tt.HasText(failure) || failure == "" {
		t.Fatal("keyboard dismissal mutated validation or failed to close")
	}
	_ = tt.Click("Save")
	tt.Frame()
	if saves != 0 {
		t.Fatal("dismissed error no longer blocks Save")
	}
}
func TestToastHasAnExplicitCloseBeforeItsTimeoutAndPreservesActions(t *testing.T) {
	emit := true
	actions := 0
	tt := ui.NewTester(func(c *ui.Context) {
		if emit {
			c.AddToast(ui.Toast{Title: "Notification", Timeout: time.Minute, Action: "Undo", OnAction: func() { actions++ }})
			emit = false
		}
		Toasts(c)
	}, 560, 400)
	if !tt.HasText("Notification") || !tt.HasText("Undo") {
		t.Fatal("native toast or action missing")
	}
	if err := tt.Click(i18n.T("notices.closeToast")); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		tt.Frame()
	}
	if tt.HasText("Notification") || actions != 0 {
		t.Fatal("manual close waited for timeout or ran Undo")
	}
}

func TestNoticeDismissalSurvivesASiblingDisappearing(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for name, show := range map[string]func(*ui.Context, string){
			"notice": func(c *ui.Context, text string) { Notice(c, Caution, "Warning", text, nil) },
			"error":  func(c *ui.Context, text string) { InlineError(c, text) },
			"status": func(c *ui.Context, text string) { Status(c, text) },
		} {
			t.Run(locale+"/"+name, func(t *testing.T) {
				sibling, message := true, "Keep this content dismissed"
				tt := ui.NewTester(func(c *ui.Context) {
					ui.Column(c).Fill().Children(func() {
						if sibling {
							show(c, "Temporary sibling")
						}
						show(c, message)
					})
				}, 560, 400)
				// Close the second feedback with its actual native keyboard
				// control, not by changing the source message or a test state.
				tt.Key(0, ui.KeyTab)
				tt.Key(0, ui.KeyTab)
				tt.Key(0, ui.KeyEnter)
				if tt.HasText(message) || !tt.HasText("Temporary sibling") {
					t.Fatal("keyboard close changed the wrong feedback")
				}
				sibling = false
				for range 5 {
					tt.Frame()
				}
				if tt.HasText(message) {
					t.Fatal("same content reappeared when its sibling changed row index")
				}
				message = "A genuinely new message"
				tt.Frame()
				if !tt.HasText(message) {
					t.Fatal("new content inherited the previous dismissal")
				}
			})
		}
	}
}
