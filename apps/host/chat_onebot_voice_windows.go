//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"strings"
	"winone/internal/ai"
	"winone/internal/audiooutput"
)

func openOneBotMicrophoneSettings() (any, error) {
	verb, _ := windows.UTF16PtrFromString("open")
	target, _ := windows.UTF16PtrFromString("ms-settings:privacy-microphone")
	err := windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL)
	return map[string]bool{"ok": err == nil}, err
}

// Credentials only cross the authenticated Worker bridge. Desktop voice.models
// is the original Cindy catalog projection and never returns this structure.
func (b *chatOneBot) voiceResources() (any, error) {
	configuration, err := b.control.aiConfiguration()
	if err != nil {
		return nil, err
	}
	b.control.ai.mu.Lock()
	store := b.control.ai.store
	b.control.ai.mu.Unlock()
	providers := []map[string]any{}
	for _, provider := range configuration.Providers {
		// Cindy speech uses BYOK HTTP providers, not another app's subscription keys.
		if provider.AuthMode == "account" || len(provider.Models) == 0 || provider.BaseURL == "" {
			continue
		}
		models := []map[string]string{}
		var key string
		for _, model := range provider.Models {
			selection := ai.Selection{Provider: provider.ID, Model: model}
			if configuration.ValidateSelection(selection) != nil {
				continue
			}
			if len(models) == 0 {
				_, key, err = store.ForTask("chat", &selection)
				if err != nil {
					return nil, err
				}
			}
			models = append(models, map[string]string{"id": model, "name": provider.ModelName(model)})
		}
		if len(models) > 0 {
			providers = append(providers, map[string]any{"id": provider.ID, "name": provider.Name, "baseUrl": provider.BaseURL, "headers": provider.Headers, "key": key, "models": models})
		}
	}
	return map[string]any{"providers": providers}, nil
}

func (b *chatOneBot) voiceRecord(ctx context.Context, raw json.RawMessage) (any, error) {
	var input struct {
		SessionID, ClientID string
		Record              json.RawMessage
		CreatedAt           int64
		Update              bool
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(input.ClientID, "voice-call:") || len(input.ClientID) > 128 || len(input.Record) > 256<<10 {
		return nil, errors.New("语音记录无效")
	}
	owner, err := b.contextFor(ctx, map[string]string{"sessionId": input.SessionID})
	if err != nil {
		return nil, err
	}
	if owner.Role != "canonical" {
		return nil, errors.New("语音记录只能保存到伙伴主对话")
	}
	if input.Update {
		if _, err = b.sessions.engine.Open(ctx, input.SessionID, 0); err != nil {
			return nil, err
		}
		defer b.sessions.engine.Release(input.SessionID, 0)
		result, err := b.sessions.engine.Command(ctx, input.SessionID, "timeline.voice", map[string]any{"clientId": input.ClientID, "record": input.Record})
		return json.RawMessage(result), err
	}
	payload, err := json.Marshal(map[string]any{"sessionId": input.SessionID, "clientId": input.ClientID, "role": "assistant", "content": "", "createdAt": input.CreatedAt, "agentMeta": map[string]any{"voiceCall": input.Record}})
	if err != nil {
		return nil, err
	}
	return b.taskNative(ctx, "timeline", payload)
}

func (b *chatOneBot) voiceSystemAudio(raw json.RawMessage) (any, error) {
	var input struct{ Muted *bool }
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	muted, err := audiooutput.Mute(input.Muted)
	return map[string]bool{"muted": muted}, err
}
