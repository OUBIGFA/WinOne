package selection

import (
	"context"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/learning"
)

func (p *page) learningAction() *action {
	for i := range p.draft.Value.Actions {
		if p.draft.Value.Actions[i].Value.Kind == "learn" {
			return &p.draft.Value.Actions[i].Value
		}
	}
	return nil
}
func (p *page) checkLearningAnki() {
	if p.learningBusy {
		return
	}
	p.learningRead++
	owner := p.learningRead
	p.learningBusy = true
	p.learningError = ""
	ctx, cancel := context.WithCancel(context.Background())
	p.learningCancel = cancel
	core.Go(p.env, func() (learning.AnkiStatus, error) { defer cancel(); return (learning.Anki{}).Status(ctx) }, func(status learning.AnkiStatus, err error) {
		if owner != p.learningRead {
			return
		}
		p.learningBusy = false
		p.learningCancel = nil
		if err != nil {
			p.learningStatus = nil
			p.learningError = err.Error()
			return
		}
		p.learningStatus = &status
	})
}
func (p *page) navigateLearning(deck string, review bool) {
	if p.learningBusy {
		return
	}
	p.learningBusy = true
	p.learningRead++
	owner := p.learningRead
	ctx, cancel := context.WithCancel(context.Background())
	p.learningCancel = cancel
	core.Do(p.env, func() error {
		defer cancel()
		if review {
			return (learning.Anki{}).Review(ctx, deck)
		}
		return (learning.Anki{}).Browse(ctx, deck, "")
	}, func(err error) {
		if owner != p.learningRead {
			return
		}
		p.learningBusy = false
		p.learningCancel = nil
		if err != nil {
			p.learningError = err.Error()
		}
	})
}
func (p *page) learningPage(c *ui.Context) {
	a := p.learningAction()
	if a == nil {
		kit.Help(c, i18n.T("learning.missing"))
		if kit.Button(c, i18n.T("learning.restore"), icons.BookOpen, false).Disabled(len(p.actions()) >= maxActions).Clicked() {
			options := learning.Defaults()
			id := "learn"
			if p.index(id) >= 0 {
				id = "custom-learn-" + uuid()
			}
			p.draft.Value.Actions = append(p.draft.Value.Actions, core.Preserved[action]{Value: action{ID: id, Name: "学习", EnglishName: "Learn", Icon: "book", Kind: "learn", Enabled: true, Learning: &options}})
		}
		return
	}
	o := a.Learning
	kit.Help(c, i18n.T("learning.intro"))
	kit.SubTabs(c, &p.learningSection, []string{i18n.T("learning.explanation"), i18n.T("learning.anki"), i18n.T("learning.templates")}, nil)
	if p.learningSection == 0 {
		kit.Group(c, icons.BookOpen, i18n.T("learning.explanation"), "", func() {
			ui.Column(c).FillWidth().Padding(16).Gap(16).Children(func() {
				ui.Field(c, i18n.T("learning.level"), func() {
					kit.Select(c, &o.Level, []kit.SelectOption{{Value: "beginner", Label: i18n.T("learning.beginner")}, {Value: "intermediate", Label: i18n.T("learning.intermediate")}, {Value: "advanced", Label: i18n.T("learning.advanced")}})
				}).FillWidth()
				ui.Field(c, i18n.T("learning.accent"), func() {
					kit.Select(c, &o.Accent, []kit.SelectOption{{Value: "us", Label: i18n.T("learning.us")}, {Value: "uk", Label: i18n.T("learning.uk")}})
				}).FillWidth()
				ui.Checkbox(c, &o.HumanVoice, i18n.T("learning.humanVoice"))
				ui.Checkbox(c, &o.OnlineDictionary, i18n.T("learning.online"))
				kit.Help(c, i18n.T("learning.privacy"))
			})
		})
	}
	if p.learningSection == 1 {
		kit.Group(c, icons.BookOpen, i18n.T("learning.anki"), "", func() {
			ui.Column(c).FillWidth().Padding(16).Gap(16).Children(func() {
				ui.Row(c).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
					if kit.Button(c, i18n.T("learning.check"), icons.RefreshCw, p.learningBusy).Disabled(p.learningBusy).Clicked() {
						p.checkLearningAnki()
					}
					if p.learningStatus != nil {
						kit.Help(c, i18n.T("learning.connected"))
					}
				})
				if p.learningError != "" {
					kit.Notice(c, kit.Failure, "", p.learningError, nil)
				}
				ui.Field(c, i18n.T("learning.deck"), func() { ui.TextInput(c, &o.Deck).FillWidth() }).FillWidth()
				if p.learningStatus != nil && len(p.learningStatus.Decks) > 0 {
					ui.Field(c, i18n.T("learning.existingDeck"), func() {
						var options []kit.SelectOption
						for _, deck := range p.learningStatus.Decks {
							options = append(options, kit.SelectOption{Value: deck, Label: deck})
						}
						kit.Select(c, &o.Deck, options)
					}).FillWidth()
				}
				ui.Field(c, i18n.T("learning.cardMode"), func() {
					kit.Select(c, &o.CardMode, []kit.SelectOption{{Value: "recall", Label: i18n.T("learning.recall")}, {Value: "typing", Label: i18n.T("learning.typing")}})
				}).FillWidth()
				ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
					if kit.Button(c, i18n.T("learning.browse"), icons.BookOpen, false).Disabled(p.learningStatus == nil || p.learningBusy).Clicked() {
						p.navigateLearning(o.Deck, false)
					}
					if kit.Button(c, i18n.T("learning.review"), icons.Play, false).Disabled(p.learningStatus == nil || p.learningBusy).Clicked() {
						p.navigateLearning(o.Deck, true)
					}
				})
				kit.Help(c, i18n.T("learning.install"))
				ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
					if ui.Button(c, i18n.T("learning.downloadAnki")).Clicked() {
						c.Services().OpenURL(learning.AnkiDownloadURL)
					}
					if ui.Button(c, i18n.T("learning.installAddon")).Clicked() {
						c.Services().OpenURL(learning.AnkiInstallURL)
					}
					if ui.Button(c, i18n.T("learning.fsrs")).Clicked() {
						c.Services().OpenURL(learning.FSRSURL)
					}
				})
				kit.Help(c, i18n.T("learning.fsrsHint"))
			})
		})
	}
	if p.learningSection == 2 {
		kit.Group(c, icons.FileText, i18n.T("learning.templates"), i18n.T("learning.templatesHint"), func() {
			ui.Column(c).FillWidth().Padding(16).Gap(16).Children(func() {
				kit.SubTabs(c, &p.learningTemplate, []string{i18n.T("learning.word"), i18n.T("learning.phrase"), i18n.T("learning.sentence"), i18n.T("learning.passage")}, nil)
				for _, kind := range []learning.Kind{[]learning.Kind{learning.Word, learning.Phrase, learning.Sentence, learning.Passage}[p.learningTemplate]} {
					ui.Field(c.Key(string(kind)), i18n.T("learning."+string(kind)), func() {
						value := o.Templates[kind]
						if value == "" {
							value = learning.DefaultTemplate(kind)
						}
						if ui.TextArea(c, &value).Lines(3, 12).Changed() {
							if o.Templates == nil {
								o.Templates = map[learning.Kind]string{}
							}
							o.Templates[kind] = value
						}
						if kit.Button(c, i18n.T("learning.resetTemplate"), icons.RotateCcw, false).Clicked() {
							delete(o.Templates, kind)
						}
					}).FillWidth()
				}
			})
		})
	}
}
