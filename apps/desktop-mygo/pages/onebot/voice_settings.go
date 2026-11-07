package onebot

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/audioio"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
	resultui "winone/resultui"
)

type SpeechSelection struct {
	ProviderID string   `json:"providerId,omitempty"`
	Agent      string   `json:"agent,omitempty"`
	ModelID    string   `json:"modelId"`
	Protocol   string   `json:"protocol,omitempty"`
	Voice      string   `json:"voice"`
	Speed      *float64 `json:"speed,omitempty"`
}
type SpeechModel struct {
	ID, Name, ProviderID, ProviderName, Agent, Protocol, DefaultVoice string
	ProtocolLocked                                                    bool
	PickerVisible                                                     *bool
	SpeedRange                                                        *struct{ Min, Max float64 }
}
type SpeechVoice struct{ ID, Name string }
type SpeechPreference struct {
	Selection        *SpeechSelection `json:"selection"`
	Customized       bool             `json:"customized"`
	SentencePlayback *bool            `json:"sentencePlayback"`
}
type SpeechWire struct {
	SampleRate    int    `json:"sampleRate"`
	Format        string `json:"format"`
	EncodedBase64 string `json:"encodedBase64"`
	Deferred      bool   `json:"deferred"`
}
type PCMChunk struct {
	PCM  string `json:"pcm"`
	Done bool   `json:"done"`
}
type voiceSettingsPane struct {
	settings                             *profileSettings
	models                               []SpeechModel
	voices                               []SpeechVoice
	preference, baseline                 SpeechPreference
	selection                            *SpeechSelection
	inherit, sentence                    bool
	loading, busy, closed, stale, edited bool
	error                                string
	previewText                          string
	previewCancel                        context.CancelFunc
	previewID                            string
}

func newVoiceSettingsPane(d *profileSettings) *voiceSettingsPane {
	p := &voiceSettingsPane{settings: d, inherit: true, sentence: true, previewText: nt("voicePreviewText")}
	p.Refresh()
	return p
}
func (p *voiceSettingsPane) Dirty() bool { return p.edited }
func (p *voiceSettingsPane) Busy() bool  { return p.busy || p.previewCancel != nil }
func (p *voiceSettingsPane) Discard() {
	p.preference = copyValue(p.baseline)
	p.selection = copyValue(p.baseline.Selection)
	p.inherit = p.selection == nil
	p.sentence = p.baseline.SentencePlayback == nil || *p.baseline.SentencePlayback
	p.edited = false
	p.error = ""
}
func (p *voiceSettingsPane) Close() { p.closed = true; p.stopPreview() }
func (p *voiceSettingsPane) Invalidate(kind string) {
	if kind == "voice-data" {
		p.stale = true
	}
}
func (p *voiceSettingsPane) Refresh() {
	if p.closed || p.loading {
		return
	}
	p.loading = true
	p.stale = false
	type result struct {
		models     []SpeechModel
		preference SpeechPreference
	}
	core.Go(p.settings.parent.env, func() (result, error) {
		var r result
		if err := p.settings.parent.call("voice.models", nil, &r.models); err != nil {
			return r, err
		}
		err := p.settings.parent.call("voice.preference", map[string]string{"botId": p.settings.botID}, &r.preference)
		return r, err
	}, func(r result, err error) {
		if p.closed {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.models = r.models
		p.preference = r.preference
		p.baseline = copyValue(r.preference)
		if !p.edited {
			p.selection = copyValue(r.preference.Selection)
			p.inherit = p.selection == nil
			p.sentence = r.preference.SentencePlayback == nil || *r.preference.SentencePlayback
		}
		p.error = ""
		p.loadVoices()
	})
}
func speechGroups(models []SpeechModel, selected *SpeechSelection) []widgets.ModelGroup {
	groups := []widgets.ModelGroup{}
	for _, model := range models {
		chosen := selected != nil && selected.ModelID == model.ID && selected.ProviderID == model.ProviderID
		if model.PickerVisible != nil && !*model.PickerVisible && !chosen {
			continue
		}
		id := model.ProviderID
		if id == "" {
			id = "default"
		}
		index := -1
		for i := range groups {
			if groups[i].ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			groups = append(groups, widgets.ModelGroup{ID: id, Name: firstNonempty(model.ProviderName, nt("defaultService"))})
			index = len(groups) - 1
		}
		value := widgets.SelectionValue(widgets.AISelection{Provider: model.ProviderID, Model: model.ID})
		groups[index].Models = append(groups[index].Models, widgets.ModelEntry{Value: value, ID: model.ID, Name: model.Name, Label: firstNonempty(model.Name, model.ID)})
	}
	return groups
}
func (p *voiceSettingsPane) model() *SpeechModel {
	if p.selection == nil {
		return nil
	}
	for i := range p.models {
		if p.models[i].ID == p.selection.ModelID && p.models[i].ProviderID == p.selection.ProviderID {
			return &p.models[i]
		}
	}
	return nil
}
func (p *voiceSettingsPane) loadVoices() {
	if p.selection == nil {
		return
	}
	selection := copyValue(*p.selection)
	oneBotCall(p.settings.parent, "voice.voices", map[string]any{"selection": selection}, func(value struct {
		Voices                             []SpeechVoice `json:"voices"`
		Truncated, CustomVoicesUnavailable bool
	}, err error) {
		if p.closed || p.selection == nil || p.selection.ModelID != selection.ModelID || p.selection.ProviderID != selection.ProviderID {
			return
		}
		if err != nil {
			p.error = err.Error()
			return
		}
		p.voices = value.Voices
		if value.CustomVoicesUnavailable {
			p.error = nt("privateVoicesUnavailable")
		}
	})
}
func (p *voiceSettingsPane) Save() {
	if p.closed || p.busy || !p.edited {
		return
	}
	p.busy = true
	selection := copyValue(p.selection)
	if p.inherit {
		selection = nil
	}
	inherit, sentence := p.inherit, p.sentence
	// Capture the retained selection too: switching to inheritance can leave
	// an editable selection to restore, and a receipt must not accept it.
	retained := copyValue(p.selection)
	oneBotCall(p.settings.parent, "voice.setPreference", map[string]any{"botId": p.settings.botID, "selection": selection, "sentencePlayback": sentence}, func(preference SpeechPreference, err error) {
		if p.closed {
			return
		}
		p.busy = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.preference, p.baseline = preference, copyValue(preference)
		p.edited = p.inherit != inherit || p.sentence != sentence || !sameJSON(p.selection, retained)
		p.error = ""
	})
}
func (p *voiceSettingsPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	if p.stale && !p.edited {
		p.Refresh()
	}
	kit.Help(c, bt("bots.voiceMode.settingsHint"))
	if p.loading {
		kit.Loader(c)
	}
	kit.InlineError(c, p.error)
	if kit.Toggle(c, &p.inherit, nt("useDefaultVoice"), "").Changed() {
		p.edited = true
		if !p.inherit && p.selection == nil {
			p.selection = &SpeechSelection{}
		}
	}
	if !p.inherit {
		selected := ""
		if p.selection != nil {
			selected = widgets.SelectionValue(widgets.AISelection{Provider: p.selection.ProviderID, Model: p.selection.ModelID})
		}
		if value, changed := widgets.ModelField(c, widgets.ModelFieldOptions{ModelMenuOptions: widgets.ModelMenuOptions{Env: p.settings.parent.env, Groups: speechGroups(p.models, p.selection), Selected: selected, Label: nt("speechModel")}, Placeholder: nt("chooseModel")}); changed {
			chosen, ok := widgets.ParseSelectionValue(value)
			if ok {
				for _, model := range p.models {
					if model.ID == chosen.Model && model.ProviderID == chosen.Provider {
						p.selection = &SpeechSelection{ProviderID: model.ProviderID, Agent: model.Agent, ModelID: model.ID, Voice: model.DefaultVoice}
						p.edited = true
						p.loadVoices()
					}
				}
			}
		}
		if p.selection != nil {
			options := []kit.SelectOption{}
			for _, voice := range p.voices {
				options = append(options, kit.SelectOption{Value: voice.ID, Label: firstNonempty(voice.Name, voice.ID)})
			}
			ui.Field(c, nt("voice"), func() {
				if changed, _ := kit.Select(c, &p.selection.Voice, options); changed {
					p.edited = true
				}
			})
			ui.Field(c, nt("manualVoiceID"), func() {
				if ui.TextInput(c, &p.selection.Voice).FillWidth().Label(nt("manualVoiceID")).Changed() {
					p.edited = true
				}
			})
			if model := p.model(); model != nil {
				if !model.ProtocolLocked {
					protocols := []kit.SelectOption{{Value: "", Label: nt("automaticProtocol")}}
					for _, protocol := range []string{"openai", "elevenlabs", "minimax", "qwen-tts", "dashscope-tts"} {
						protocols = append(protocols, kit.SelectOption{Value: protocol, Label: protocol})
					}
					ui.Field(c, nt("speechProtocol"), func() {
						if changed, _ := kit.Select(c, &p.selection.Protocol, protocols); changed {
							p.edited = true
						}
					})
				}
				if model.SpeedRange != nil {
					speed := 1.0
					if p.selection.Speed != nil {
						speed = *p.selection.Speed
					}
					ui.Field(c, nt("speechSpeed"), func() {
						if ui.Slider(c, &speed, model.SpeedRange.Min, model.SpeedRange.Max).FillWidth().Label(nt("speechSpeed")).Changed() {
							p.selection.Speed = &speed
							p.edited = true
						}
					})
				}
			}
		}
	}
	if kit.Toggle(c, &p.sentence, nt("sentencePlayback"), nt("sentencePlaybackHint")).Changed() {
		p.edited = true
	}
	ui.TextInput(c, &p.previewText).FillWidth().Label(nt("previewText"))
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if kit.PrimaryButton(c, nt("save"), icons.Save, p.busy).Disabled(!p.edited || p.busy).Clicked() {
			p.Save()
		}
		if kit.Button(c, nt("voicePreview"), icons.Play, false).Disabled(p.previewCancel != nil || p.selection == nil || p.edited).Clicked() {
			p.preview()
		}
		if p.previewCancel != nil && kit.Button(c, nt("stop"), icons.Square, false).Clicked() {
			p.stopPreview()
		}
		if kit.Button(c, nt("refresh"), icons.RefreshCw, p.loading).Clicked() {
			p.Refresh()
		}
		if kit.Button(c, nt("configureAI"), icons.Settings2, false).Clicked() {
			p.settings.parent.env.Navigate("ai-services", nil)
		}
	})
}
func playSpeechWire(ctx context.Context, wire SpeechWire, read func() (PCMChunk, error)) error {
	if wire.Deferred {
		return nil
	}
	if wire.Format == "mp3" {
		bytes, err := base64.StdEncoding.DecodeString(wire.EncodedBase64)
		if err != nil {
			return err
		}
		return resultui.PlayEncodedSpeech(ctx, bytes)
	}
	out, err := audioio.OpenOutput(ctx, wire.SampleRate)
	if err != nil {
		return err
	}
	defer out.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk, err := read()
		if err != nil {
			return err
		}
		data, err := base64.StdEncoding.DecodeString(chunk.PCM)
		if err != nil {
			return err
		}
		if err = out.Write(data); err != nil {
			return err
		}
		if chunk.Done {
			return nil
		}
		if len(data) == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(40 * time.Millisecond):
			}
		}
	}
}
func (p *voiceSettingsPane) preview() {
	if p.selection == nil || p.previewCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.previewCancel = cancel
	p.previewID = rand.Text()
	id := p.previewID
	selection := copyValue(*p.selection)
	core.Do(p.settings.parent.env, func() error {
		input := map[string]any{"previewId": id, "botId": p.settings.botID, "sessionId": canonical(p.settings.draft), "text": p.previewText}
		raw, _ := json.Marshal(selection)
		var fields map[string]any
		_ = json.Unmarshal(raw, &fields)
		for key, value := range fields {
			input[key] = value
		}
		var wire SpeechWire
		if err := p.settings.parent.call("voice.preview", map[string]any{"input": input}, &wire); err != nil {
			return err
		}
		defer p.settings.parent.call("voice.stopPreview", map[string]string{"previewId": id}, nil)
		return playSpeechWire(ctx, wire, func() (PCMChunk, error) {
			var chunk PCMChunk
			err := p.settings.parent.call("voice.readPreview", map[string]string{"previewId": id}, &chunk)
			return chunk, err
		})
	}, func(err error) {
		if p.closed || p.previewID != id {
			return
		}
		p.previewCancel = nil
		if err != nil && ctx.Err() == nil {
			p.error = nt("playbackFailed") + " · " + fmt.Sprint(err)
		}
	})
}
func (p *voiceSettingsPane) stopPreview() {
	if p.previewCancel != nil {
		p.previewCancel()
		p.previewCancel = nil
	}
	id := p.previewID
	p.previewID = ""
	if id != "" {
		core.Do(p.settings.parent.env, func() error {
			return p.settings.parent.call("voice.stopPreview", map[string]string{"previewId": id}, nil)
		}, func(error) {})
	}
}
