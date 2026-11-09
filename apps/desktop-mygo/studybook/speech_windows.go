package studybook

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
	"runtime"
	"strings"
	"unsafe"
	"winone/desktopmygo/core"
)

type speechItem struct {
	Text, Language, Side string
	Speed                float64
	Voices               []string
}

// Mirrors Anki's WindowsTTSPlayer: native SAPI voices, the same speed-to-rate
// conversion, plain text, asynchronous playback, and cancellable completion.
func speakNative(ctx context.Context, items []speechItem) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		return err
	}
	defer ole.CoUninitialize()
	unknown, err := oleutil.CreateObject("SAPI.SpVoice")
	if err != nil {
		return err
	}
	defer unknown.Release()
	voice, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer voice.Release()
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		locale, err := windows.UTF16PtrFromString(strings.ReplaceAll(item.Language, "_", "-"))
		if err != nil {
			return err
		}
		lcid, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("LocaleNameToLCID").Call(uintptr(unsafe.Pointer(locale)), 0)
		if lcid == 0 {
			return fmt.Errorf("%s: %s", text("voiceUnavailable"), item.Language)
		}
		voices, err := oleutil.CallMethod(voice, "GetVoices", fmt.Sprintf("Language=%x", lcid), "")
		if err != nil {
			return err
		}
		list := voices.ToIDispatch()
		count, err := oleutil.GetProperty(list, "Count")
		if err != nil {
			voices.Clear()
			return err
		}
		total := int(count.Val)
		count.Clear()
		var chosen *ole.VARIANT
		for index := 0; index < total; index++ {
			candidate, e := oleutil.CallMethod(list, "Item", index)
			if e != nil {
				voices.Clear()
				return e
			}
			match := len(item.Voices) == 0
			if !match {
				name, e := oleutil.CallMethod(candidate.ToIDispatch(), "GetAttribute", "Name")
				if e == nil {
					normalized := strings.ReplaceAll(strings.TrimSuffix(name.ToString(), " Desktop"), " ", "_")
					for _, requested := range item.Voices {
						match = match || requested == normalized
					}
					name.Clear()
				}
			}
			if match {
				chosen = candidate
				break
			}
			candidate.Clear()
		}
		if chosen == nil {
			voices.Clear()
			return fmt.Errorf("%s: %s", text("voiceUnavailable"), item.Language)
		}
		_, err = oleutil.PutProperty(voice, "Voice", chosen.ToIDispatch())
		chosen.Clear()
		voices.Clear()
		if err != nil {
			return err
		}
		rate := int(max(-10.0, min(10.0, item.Speed*10-10)))
		if _, err = oleutil.PutProperty(voice, "Rate", rate); err != nil {
			return err
		}
		value, err := oleutil.CallMethod(voice, "Speak", item.Text, 17)
		if err != nil {
			return err
		}
		value.Clear()
		for {
			if ctx.Err() != nil {
				value, _ := oleutil.CallMethod(voice, "Speak", "", 3)
				if value != nil {
					value.Clear()
				}
				return ctx.Err()
			}
			done, err := oleutil.CallMethod(voice, "WaitUntilDone", 100)
			if err != nil {
				return err
			}
			finished, _ := done.Value().(bool)
			done.Clear()
			if finished {
				break
			}
		}
	}
	return nil
}
func (b *Book) stopSpeech() {
	if b.speechCancel != nil {
		b.speechCancel()
		b.speechCancel = nil
	}
	b.speaking = false
}
func (b *Book) playSpeech() {
	if b.review == nil {
		return
	}
	b.stopSpeech()
	var items []speechItem
	for _, item := range b.review.Speech {
		if item.Side == "q" && b.review.Side == "question" || b.review.Side == "answer" && (item.Side == "a" || b.review.ReplayQuestion) {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.speechCancel = cancel
	b.speaking = true
	core.Do(b.env, func() error { return speakNative(ctx, items) }, func(err error) {
		if b.closed || ctx.Err() != nil {
			return
		}
		b.speaking = false
		b.speechCancel = nil
		cancel()
		if err != nil && !errors.Is(err, context.Canceled) {
			b.err = err.Error()
		}
	})
}
