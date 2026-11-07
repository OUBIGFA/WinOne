package onebot

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"winone/desktopmygo/chat/mathview"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"winone/desktopmygo/audioio"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	resultui "winone/resultui"
)

type VoiceTurn struct {
	ID, Role, Text string
	Complete       bool
	CreatedAt      int64
}
type VoiceHandoff struct{ ID, InputID, Text, State string }
type VoiceRecord struct {
	CallID    string
	Turns     []VoiceTurn
	Handoffs  []VoiceHandoff
	StartedAt int64
}
type voiceEvent struct {
	CallID          string `json:"callId"`
	InputGeneration int    `json:"inputGeneration"`
	Event           struct {
		Type, ID, Text string
		Record         VoiceRecord
	}
}
type voiceSession struct {
	parent                            *page
	botID, sessionID, callID          string
	selection                         SpeechSelection
	sentence                          bool
	ctx                               context.Context
	cancel                            context.CancelFunc
	mic                               *audioio.Input
	micCancel                         context.CancelFunc
	deviceID                          string
	inputGeneration                   int
	phase, error, unsent, lastInputID string
	muted, busy, closed, ready        bool
	blocked, hearing                  bool
	record                            VoiceRecord
	activityAt                        time.Time
	exitAt                            time.Time
	tickCancel                        func()
	speechText                        map[string]string
	speechRunning                     bool
	speechCancel                      context.CancelFunc
	speechQueue                       []VoiceTurn
	currentTurn, currentRequest       string
}

func resolveSpeechSelection(preference *SpeechSelection, models []SpeechModel) (SpeechSelection, error) {
	if preference != nil {
		for _, model := range models {
			if model.ID == preference.ModelID && model.ProviderID == preference.ProviderID {
				return copyValue(*preference), nil
			}
		}
		return SpeechSelection{}, fmt.Errorf("%s", nt("speechModelUnavailable"))
	}
	for _, model := range models {
		if model.PickerVisible != nil && !*model.PickerVisible {
			continue
		}
		return SpeechSelection{ProviderID: model.ProviderID, Agent: model.Agent, ModelID: model.ID, Voice: model.DefaultVoice}, nil
	}
	return SpeechSelection{}, fmt.Errorf("%s", nt("speechModelUnavailable"))
}
func (p *page) startVoice(botID, sessionID string) {
	if p.voice != nil && !p.voice.closed {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	v := &voiceSession{parent: p, botID: botID, sessionID: sessionID, callID: rand.Text(), ctx: ctx, cancel: cancel, phase: "connecting", busy: true, sentence: true, speechText: map[string]string{}, activityAt: time.Now()}
	p.voice = v
	type started struct {
		selection SpeechSelection
		deviceID  string
		sentence  bool
	}
	core.Go(p.env, func() (started, error) {
		var r started
		var preference SpeechPreference
		if err := p.call("voice.preference", map[string]string{"botId": botID}, &preference); err != nil {
			return r, err
		}
		var models []SpeechModel
		if err := p.call("voice.models", nil, &models); err != nil {
			return r, err
		}
		selection, err := resolveSpeechSelection(preference.Selection, models)
		if err != nil {
			return r, err
		}
		if selection.Voice == "" {
			var catalogue struct {
				Voices []SpeechVoice `json:"voices"`
			}
			if err = p.call("voice.voices", map[string]any{"selection": selection}, &catalogue); err != nil {
				return r, err
			}
			if len(catalogue.Voices) == 0 {
				return r, fmt.Errorf("%s", nt("voiceRequired"))
			}
			selection.Voice = catalogue.Voices[0].ID
		}
		r.selection = selection
		r.sentence = preference.SentencePlayback == nil || *preference.SentencePlayback
		var inputData struct {
			Settings struct {
				MicrophoneDeviceID *string `json:"microphoneDeviceId"`
			} `json:"settings"`
		}
		if err = p.call("voice.input.data", nil, &inputData); err != nil {
			return r, err
		}
		if inputData.Settings.MicrophoneDeviceID != nil {
			r.deviceID = *inputData.Settings.MicrophoneDeviceID
		}
		raw, _ := json.Marshal(selection)
		var input map[string]any
		_ = json.Unmarshal(raw, &input)
		input["callId"], input["botId"], input["sessionId"] = v.callID, botID, sessionID
		if err = p.call("voice.start", map[string]any{"input": input}, nil); err != nil {
			return r, err
		}
		return r, nil
	}, func(r started, err error) {
		if p.voice != v || v.closed {
			return
		}
		v.busy = false
		if err != nil {
			v.error = err.Error()
			v.phase = "error"
			return
		}
		v.selection, v.deviceID, v.sentence = r.selection, r.deviceID, r.sentence
		v.ready = true
		v.startMic()
		v.tick()
	})
}
func (v *voiceSession) startMic() {
	if v.closed || v.muted || v.blocked {
		return
	}
	if v.micCancel != nil {
		v.micCancel()
	}
	ctx, cancel := context.WithCancel(v.ctx)
	v.micCancel = cancel
	generation := v.inputGeneration
	id := v.callID
	core.Go(v.parent.env, func() (*audioio.Input, error) {
		return audioio.StartInput(ctx, v.deviceID, func(bytes []byte) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return v.parent.call("voice.audio", map[string]any{"callId": id, "pcm": base64.StdEncoding.EncodeToString(bytes), "inputGeneration": generation}, nil)
		}, func(err error) {
			v.parent.env.Post(func() {
				if !v.closed {
					v.phase = "error"
					v.error = nt("microphoneFailed") + " · " + err.Error()
					v.muted = true
				}
			})
		})
	}, func(input *audioio.Input, err error) {
		if v.closed || ctx.Err() != nil {
			if input != nil {
				input.Stop()
			}
			return
		}
		if err != nil {
			v.phase = "error"
			v.error = nt("microphoneFailed") + " · " + err.Error()
			return
		}
		v.mic = input
		v.phase = "listening"
	})
}
func (p *page) receiveVoice(raw json.RawMessage) {
	v := p.voice
	if v == nil || v.closed {
		return
	}
	var event voiceEvent
	if json.Unmarshal(raw, &event) != nil || event.CallID != v.callID {
		return
	}
	if event.Event.Type != "call-record" && event.Event.Type != "ended" && event.InputGeneration != v.inputGeneration {
		return
	}
	switch event.Event.Type {
	case "ready":
		v.phase = "listening"
	case "speech-started":
		v.activityAt = time.Now()
		v.exitAt = time.Time{}
		v.interrupt()
		v.phase = "listening"
	case "draft":
		v.unsent = event.Event.Text
		v.hearing = strings.TrimSpace(v.unsent) != ""
		if strings.TrimSpace(v.unsent) != "" {
			v.activityAt = time.Now()
			v.exitAt = time.Time{}
		}
	case "utterance":
		v.activityAt = time.Now()
		v.hearing = false
		v.lastInputID = event.Event.ID
		// While the agent's question waits, speech answers that card only: it can
		// never become a task or an answer to a permission request.
		if p.chat != nil && p.chat.VoiceQuestionPending() {
			if !p.chat.ReceiveVoiceAnswer(event.Event.Text) {
				v.unsent = event.Event.Text
			} else {
				v.unsent = ""
			}
			break
		}
		v.unsent = event.Event.Text
		v.submit(event.Event.ID, event.Event.Text)
	case "error":
		v.error = event.Event.Text
		v.phase = "error"
		if v.micCancel != nil {
			v.micCancel()
		}
		v.muted = true
	case "ended":
		v.Close(false)
	case "speaker-filtered":
		v.error = nt("speakerFiltered")
	case "call-record":
		v.record = event.Event.Record
		for _, turn := range v.record.Turns {
			if turn.Role == "assistant" && turn.Text != "" && (v.sentence || turn.Complete) {
				v.say(turn)
			}
		}
	}
}
func (v *voiceSession) submit(inputID, text string) {
	if v.closed || v.busy || strings.TrimSpace(text) == "" {
		return
	}
	v.busy = true
	v.phase = "thinking"
	oneBotCall(v.parent, "voice.submitTurn", map[string]any{"callId": v.callID, "inputId": inputID, "text": text, "forceHandoff": false}, func(accepted bool, err error) {
		if v.closed {
			return
		}
		v.busy = false
		if err != nil || !accepted {
			if err != nil {
				v.error = err.Error()
			} else {
				v.error = nt("voiceSubmitFailed")
			}
			v.unsent = text
			return
		}
		v.unsent = ""
		v.activityAt = time.Now()
		oneBotCall(v.parent, "voice.classifyExit", map[string]any{"callId": v.callID, "utteranceId": inputID, "inputGeneration": v.inputGeneration}, func(result json.RawMessage, err error) {
			if v.closed || err != nil {
				return
			}
			if string(result) == "true" {
				v.exitAt = time.Now().Add(3 * time.Second)
			} else if string(result) == `"submit_then_exit"` {
				v.Close(true)
			}
		})
	})
}
func (v *voiceSession) say(turn VoiceTurn) {
	if v.closed || v.speechText[turn.ID] == turn.Text {
		return
	}
	v.speechText[turn.ID] = turn.Text
	v.activityAt = time.Now()
	v.exitAt = time.Time{}
	if v.speechRunning && v.currentTurn != turn.ID {
		for i := range v.speechQueue {
			if v.speechQueue[i].ID == turn.ID {
				v.speechQueue[i] = turn
				return
			}
		}
		v.speechQueue = append(v.speechQueue, turn)
		return
	}
	requestID := v.callID + ":" + turn.ID
	if v.currentTurn == turn.ID {
		requestID = v.currentRequest
	}
	input := map[string]any{"callId": v.callID, "requestId": requestID, "text": resultui.SpeechText(turn.Text, true), "streaming": !turn.Complete}
	oneBotCall(v.parent, "voice.speak", map[string]any{"input": input}, func(wire SpeechWire, err error) {
		if v.closed {
			return
		}
		if err != nil {
			v.error = nt("playbackFailed") + " · " + err.Error()
			return
		}
		if v.speechRunning || wire.Deferred {
			return
		}
		v.speechRunning = true
		v.currentTurn, v.currentRequest = turn.ID, requestID
		v.phase = "speaking"
		ctx, cancel := context.WithCancel(v.ctx)
		v.speechCancel = cancel
		core.Do(v.parent.env, func() error {
			return playSpeechWire(ctx, wire, func() (PCMChunk, error) {
				var chunk PCMChunk
				err := v.parent.call("voice.readSpeech", map[string]string{"callId": v.callID, "requestId": requestID}, &chunk)
				return chunk, err
			})
		}, func(err error) {
			if v.closed || v.currentRequest != requestID {
				return
			}
			v.speechRunning = false
			v.speechCancel = nil
			v.phase = "listening"
			v.activityAt = time.Now()
			if err != nil && ctx.Err() == nil {
				v.error = nt("playbackFailed") + " · " + err.Error()
			}
			if len(v.speechQueue) > 0 {
				next := v.speechQueue[0]
				v.speechQueue = v.speechQueue[1:]
				delete(v.speechText, next.ID)
				v.say(next)
			}
		})
	})
}
func (v *voiceSession) interrupt() {
	if v.closed {
		return
	}
	if v.speechCancel != nil {
		v.speechCancel()
	}
	v.speechQueue = nil
	v.currentTurn, v.currentRequest = "", ""
	v.speechRunning = false
	core.Do(v.parent.env, func() error { return v.parent.call("voice.interrupt", map[string]string{"callId": v.callID}, nil) }, func(err error) {
		if !v.closed && err != nil {
			v.error = err.Error()
		}
	})
}
func (v *voiceSession) mute() {
	if v.closed || v.busy {
		return
	}
	muted := !v.muted
	v.busy = true
	if muted && v.micCancel != nil {
		v.micCancel()
	}
	oneBotCall(v.parent, "voice.setInputMuted", map[string]any{"callId": v.callID, "muted": muted}, func(result struct {
		InputGeneration int `json:"inputGeneration"`
	}, err error) {
		if v.closed {
			return
		}
		v.busy = false
		if err != nil {
			v.error = err.Error()
			return
		}
		v.muted = muted
		v.inputGeneration = result.InputGeneration
		if !muted {
			v.startMic()
		} else {
			v.phase = "paused"
		}
	})
}
func (v *voiceSession) tick() {
	if v.closed {
		return
	}
	now := time.Now()
	if v.phase == "listening" && !v.busy && !v.speechRunning && v.unsent == "" && now.Sub(v.activityAt) >= time.Minute && v.exitAt.IsZero() {
		v.exitAt = now.Add(10 * time.Second)
	}
	if !v.exitAt.IsZero() && !now.Before(v.exitAt) {
		v.Close(true)
		return
	}
	v.tickCancel = v.parent.after(time.Second, v.tick)
}
func (v *voiceSession) Close(endHost bool) {
	if v.closed {
		return
	}
	v.closed = true
	v.cancel()
	if v.micCancel != nil {
		v.micCancel()
	}
	if v.mic != nil {
		v.mic.Stop()
	}
	if v.tickCancel != nil {
		v.tickCancel()
	}
	if endHost {
		core.Do(v.parent.env, func() error { return v.parent.call("voice.end", map[string]string{"callId": v.callID}, nil) }, func(err error) {
			if err != nil && !v.parent.closed {
				v.parent.error = err.Error()
			}
		})
	}
}
func (p *page) closeVoice() {
	if p.voice != nil {
		p.voice.Close(true)
		p.voice = nil
	}
}
func (p *page) receiveVoiceInput(raw json.RawMessage) {
	if p.dictation != nil {
		p.dictation.receive(raw)
	}
}

// voicePanel is the open call, above the composer, as Cindy's CompanionVoicePanel:
// the transcript and any question stay reachable while it runs.
func (p *page) voicePanel(c *ui.Context) {
	v := p.voice
	if v == nil || v.closed {
		return
	}
	t, pal := c.Theme(), kit.Colors(c)
	ui.Column(c.Key("voice-call")).FillWidth().MaxHeight(360).Gap(8).Padding(10, 12).Radius(14).Border(1, t.Border).Background(pal.Panel).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			kit.Strong(c, nt("voiceCall")).Shrink(0)
			kit.Status(c, nt("voicePhase."+v.phase)).Grow(1).MinWidth(0)
			if kit.Button(c, nt("endCall"), icons.X, false).Clicked() {
				p.closeVoice()
			}
		})
		kit.InlineError(c, v.error)
		ui.Scroll(c.Key("turns")).FillWidth().Grow(1).Basis(0).MinHeight(60).Gap(8).Children(func() {
			for _, turn := range v.record.Turns {
				ui.Column(c.Key(turn.ID)).FillWidth().Gap(4).Children(func() {
					kit.Caption(c, nt("voiceRole."+turn.Role))
					markdown.View(c, turn.Text, mathview.For(p.env).Apply(&markdown.Options{})).FillWidth()
					if turn.Role == "user" && kit.Button(c, nt("handoffVoice"), icons.ArrowUpRight, false).Disabled(v.busy).Clicked() {
						oneBotCall(p, "voice.promoteTurn", map[string]string{"callId": v.callID, "inputId": turn.ID}, func(accepted bool, err error) {
							if v.closed {
								return
							}
							if err != nil {
								v.error = err.Error()
							} else if !accepted {
								v.error = nt("voiceSubmitFailed")
							}
						})
					}
				})
			}
			for _, handoff := range v.record.Handoffs {
				kit.Status(c, handoff.Text+" · "+nt("voiceHandoff."+handoff.State))
			}
		})
		ui.TextArea(c, &v.unsent).FillWidth().Height(60).Label(nt("voiceDraft"))
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			if kit.Button(c, nt("muteMic"), icons.MicOff, false).Disabled(!v.ready || v.busy).Clicked() {
				v.mute()
			}
			if kit.Button(c, nt("interruptSpeech"), icons.Square, false).Clicked() {
				v.interrupt()
			}
			if kit.PrimaryButton(c, nt("send"), icons.SendHorizontal, v.busy).Disabled(!v.ready || v.busy).Clicked() {
				v.send()
			}
		})
		if !v.exitAt.IsZero() {
			kit.Status(c, nt("voiceExitCountdown", "seconds", max(0, int(time.Until(v.exitAt).Seconds())+1)))
			if kit.Button(c, nt("keepCall"), icons.Play, false).Clicked() {
				v.exitAt = time.Time{}
				v.activityAt = time.Now()
			}
		}
	})
}

// send finishes the utterance being heard, or retries what could not be sent.
func (v *voiceSession) send() {
	if v.hearing {
		v.hearing = false
		oneBotCall(v.parent, "voice.finishUtterance", map[string]string{"callId": v.callID}, func(_ json.RawMessage, err error) {
			if err != nil && !v.closed {
				v.error = err.Error()
			}
		})
		return
	}
	id := v.lastInputID
	if id == "" {
		id = rand.Text()
	}
	v.submit(id, v.unsent)
}

// setBlocked pauses the microphone during an approval or a suspended system and
// keeps what was heard as the draft. Unblocking starts capture only for a call
// that was listening: a suspended system stays paused until the user acts.
func (v *voiceSession) setBlocked(blocked bool) {
	if v.closed || blocked == v.blocked {
		return
	}
	v.blocked = blocked
	if blocked {
		if v.micCancel != nil {
			v.micCancel()
			v.micCancel = nil
		}
		v.phase = "paused"
		return
	}
	if v.ready && !v.muted {
		v.startMic()
	}
}
