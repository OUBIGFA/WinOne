package resultui

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/egoist/mygo/ui"
	"testing"
	"winone/internal/ai"
)

func TestReplyShowsProviderAndModelFromRequestSnapshot(t *testing.T) {
	c := &Controller{Panel: NewPanel("ocr", "zh-CN", "light"), ctx: context.Background()}
	var value Outcome
	if err := json.Unmarshal([]byte(`{"text":"Answer","model":"model-a","providerName":"Personal service"}`), &value); err != nil {
		t.Fatal(err)
	}
	c.acceptReply(value, false)
	// Settings may have been renamed or switched since this request started.
	c.Panel.Configuration = ai.Configuration{Providers: []ai.Provider{{ID: "one", Name: "Renamed service", Models: []string{"model-b"}}}}
	c.Panel.Selection = &ai.Selection{Provider: "one", Model: "model-b"}
	tt := ui.NewTester(c.Panel.View, 420, 500)
	provider, found := tt.Find("Personal service")
	model, modelFound := tt.Find("model-a")
	if !found || !modelFound || model.X <= provider.X || provider.Y >= model.Y+model.H || model.Y >= provider.Y+provider.H {
		t.Fatal("reply must retain its request provider beside its model")
	}
}

func TestCompletedRequestReleasesContextWithoutAffectingNewerRequest(t *testing.T) {
	c := &Controller{Panel: NewPanel("ocr", "zh-CN", "light"), ctx: context.Background()}
	old, epoch := c.begin("chat")
	current, next := c.begin("chat")
	c.completeRequest(epoch, func() { t.Fatal("stale result applied") }, nil)
	if old.Err() == nil || current.Err() != nil || !c.Panel.Busy {
		t.Fatal("stale completion affected current request")
	}
	applied := false
	c.completeRequest(next, func() { applied = true }, nil)
	if !applied || current.Err() != context.Canceled || c.Panel.Busy {
		t.Fatal("completed request retained its context")
	}
}

func TestRetryPreservesAnswerUntilReplacementSucceeds(t *testing.T) {
	c := &Controller{Panel: NewPanel("ocr", "zh-CN", "light"), ctx: context.Background()}
	c.Panel.Original, c.Panel.Result = "Source", "Source"
	c.history = []ai.Message{{Role: "user", Content: "Question"}, {Role: "assistant", Content: "Previous answer"}}
	c.Panel.Messages = []Message{{Role: "user", Text: "Question"}, {Role: "assistant", Text: "Previous answer"}}
	retry := c.retryHistory()
	if len(retry) != 3 || retry[2].Role != "user" || retry[2].Content != "Question" {
		t.Fatalf("retry lost context: %+v", retry)
	}
	_, epoch := c.begin("chat")
	c.completeRequest(epoch, func() { c.acceptReply(Outcome{Text: "Replacement", Model: "model-b"}, true) }, errors.New("service unavailable"))
	if len(c.history) != 2 || c.history[1].Content != "Previous answer" || c.Panel.Messages[1].Text != "Previous answer" {
		t.Fatal("failed retry discarded answer")
	}
	_, epoch = c.begin("chat")
	c.completeRequest(epoch, func() { c.acceptReply(Outcome{Text: "Replacement", Model: "model-b"}, true) }, nil)
	if len(c.history) != 2 || len(c.Panel.Messages) != 2 || c.history[1].Content != "Replacement" || c.Panel.Messages[1].Text != "Replacement" {
		t.Fatal("successful retry did not replace answer")
	}
	if c.Panel.Messages[1].Model != "model-b" {
		t.Fatal("reply lost the model that generated it")
	}
}
