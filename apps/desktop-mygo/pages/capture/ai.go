package capture

import (
	"encoding/json"
	"maps"
	"reflect"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// aiChoices are the parts of the shared AI configuration this tab edits:
// which engines recognize and translate, the models of the three capture
// tasks, the target language and the translation prompt. Providers, keys
// and models belong to the AI services page; the rest of the document is
// sent back as it was read.
type aiChoices struct {
	Tasks     map[string]widgets.AISelection `json:"tasks"`
	Engine    string                         `json:"engine"`
	OCREngine string                         `json:"ocrEngine"`
	Target    string                         `json:"target"`
	Prompt    string                         `json:"prompt"`
}

type aiDocument = core.Preserved[aiChoices]

// aiSection is the Recognition & translation tab (React's
// CaptureAISettings). It is made when the tab first shows and kept, so
// its draft survives switching tabs.
type aiSection struct {
	env          *core.Env
	saved, draft *aiDocument
	providers    []widgets.AIProvider
	loading      bool
	working      bool
	read         int
	err          string
}

func newAISection(env *core.Env) *aiSection {
	a := &aiSection{env: env}
	a.load()
	return a
}

// aiRead is a configuration as read: the document to edit and the
// providers the model pickers offer.
type aiRead struct {
	document  aiDocument
	providers []widgets.AIProvider
}

func decodeAI(raw json.RawMessage) (aiRead, error) {
	var r aiRead
	if err := json.Unmarshal(raw, &r.document); err != nil {
		return r, err
	}
	var public struct {
		Providers []widgets.AIProvider `json:"providers"`
	}
	if err := json.Unmarshal(raw, &public); err != nil {
		return r, err
	}
	r.providers = public.Providers
	return r, nil
}

// load reads the configuration, dropping the draft; Undo and discarding
// changes read it again too.
func (a *aiSection) load() {
	a.read++
	owner := a.read
	a.loading, a.err = true, ""
	core.Go(a.env, func() (aiRead, error) {
		raw, err := a.env.Host.CallCompleting("capture.ai.configuration.get", nil)
		if err != nil {
			return aiRead{}, err
		}
		return decodeAI(raw)
	}, func(r aiRead, err error) {
		if owner != a.read {
			return
		}
		a.loading = false
		if err != nil {
			a.err = err.Error()
			return
		}
		a.accept(r)
	})
}

func (a *aiSection) accept(r aiRead) {
	saved, draft := r.document, r.document
	draft.Value.Tasks = maps.Clone(saved.Value.Tasks)
	a.saved, a.draft, a.providers = &saved, &draft, r.providers
}

func (a *aiSection) dirty() bool {
	return a.draft != nil && a.saved != nil && !reflect.DeepEqual(a.draft.Value, a.saved.Value)
}

func (a *aiSection) busy() bool { return a.working || a.loading }

// report records the tab's draft and save, whether or not it shows.
func (a *aiSection) report() {
	a.env.Activity.Report("capture-ai", a.dirty(), a.working, a.load)
}

func (a *aiSection) save() {
	if a.draft == nil || a.busy() {
		return
	}
	a.working, a.err = true, ""
	draft := *a.draft
	core.Go(a.env, func() (aiRead, error) {
		raw, err := a.env.Host.CallCompleting("capture.ai.configuration.save", map[string]any{"configuration": draft})
		if err != nil {
			return aiRead{}, err
		}
		return decodeAI(raw)
	}, func(r aiRead, err error) {
		a.working = false
		if err != nil {
			a.err = err.Error()
			return
		}
		a.read++ // a read still on its way is older than this answer
		a.loading = false
		a.accept(r)
		a.env.Toast(i18n.T("common.saved"))
	})
}

func (a *aiSection) view(c *ui.Context) {
	dirty := a.dirty()
	if a.err != "" {
		kit.Notice(c, kit.Failure, "", a.err, func() {
			if !dirty && kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, a.loading).Disabled(a.busy()).Clicked() {
				a.load()
			}
		})
	}
	if a.draft == nil {
		if a.err == "" {
			ui.Column(c).FillWidth().Gap(16).Children(func() {
				kit.Status(c, i18n.T("capture.loading"))
				kit.Skeleton(c, 5)
			})
		}
		return
	}
	a.env.Rail.Set(
		core.Action{Label: i18n.T("capture.save"), Disabled: !dirty || a.busy(), Busy: a.working, Run: a.save},
		core.Action{Disabled: !dirty || a.busy(), Run: a.load},
	)
	d := &a.draft.Value
	ui.Column(c).FillWidth().Gap(kit.GroupGap).Disabled(a.busy()).Children(func() {
		kit.Group(c, icons.ScanText, i18n.T("ai.recognitionGroup"), "", func() {
			kit.Row(c, i18n.T("ai.ocrEngine"), "", func() {
				kit.Segmented(c, &d.OCREngine, []kit.SelectOption{
					{Value: "local", Label: i18n.T("ai.localOCR")},
					{Value: "ai", Label: i18n.T("ai.sharedModel")},
				}, i18n.T("ai.ocrEngine"))
			})
			if d.OCREngine == "ai" {
				kit.Stack(c, "", "", func() {
					a.task(c, "ocr")
					kit.Help(c, i18n.T("ai.visionHint"))
				})
			}
		})
		kit.Group(c, icons.Languages, i18n.T("ai.translationGroup"), "", func() {
			help := ""
			if d.Engine != "llm" {
				help = i18n.T("ai.online")
			}
			kit.Row(c, i18n.T("ai.engine"), help, func() {
				kit.Segmented(c, &d.Engine, []kit.SelectOption{
					{Value: "google", Label: i18n.T("ai.builtin")},
					{Value: "llm", Label: i18n.T("ai.sharedModel")},
				}, i18n.T("ai.engine"))
			})
			if d.Engine == "llm" {
				kit.Stack(c, "", "", func() { a.task(c, "translate") })
			}
			kit.Row(c, i18n.T("ai.target"), "", func() {
				_, trigger := kit.Select(c, &d.Target, targets())
				trigger.Label(i18n.T("ai.target"))
			})
			if d.Engine == "llm" {
				kit.Stack(c, "", "", func() {
					ui.Field(c, i18n.T("ai.prompt"), func() {
						ui.TextArea(c, &d.Prompt).Placeholder(i18n.T("ai.promptDefault")).Lines(3, 12)
					}).Description(i18n.T("ai.promptHint"))
				})
			}
		})
		kit.Group(c, icons.MessagesSquare, i18n.T("ai.conversationGroup"), "", func() {
			kit.Stack(c, "", "", func() { a.task(c, "chat") })
		})
	})
}

// targets are the languages translation can produce.
func targets() []kit.SelectOption {
	var options []kit.SelectOption
	for _, target := range [][2]string{{"auto", "auto"}, {"zh-CN", "chinese"}, {"zh-TW", "traditional"}, {"en", "english"}, {"ja", "japanese"}, {"ko", "korean"}} {
		options = append(options, kit.SelectOption{Value: target[0], Label: i18n.T("ai." + target[1])})
	}
	return options
}

// task chooses the model of one capture task: "ocr", "translate" or
// "chat". Following the default service removes the task's entry.
func (a *aiSection) task(c *ui.Context, task string) {
	d := &a.draft.Value
	var value *widgets.AISelection
	if selection, ok := d.Tasks[task]; ok {
		value = &selection
	}
	next, changed := widgets.AIModelSelect(c, widgets.AIModelSelectOptions{
		Env: a.env, Providers: a.providers, Value: value, Label: i18n.T("ai." + task + "Task"),
		InheritLabel: i18n.T("ai.followDefaultService"), Disabled: a.busy(),
	})
	if !changed {
		return
	}
	if next == nil {
		delete(d.Tasks, task)
		return
	}
	if d.Tasks == nil {
		d.Tasks = map[string]widgets.AISelection{}
	}
	d.Tasks[task] = *next
}
