package core

import "sync"

// voicePause reaches the pages holding a microphone when the system suspends
// or the workstation locks: the window sees those messages, the page owns the
// capture. Resuming or unlocking never starts a microphone again.
var voicePause = struct {
	sync.Mutex
	next      int
	listeners map[int]func()
}{listeners: map[int]func(){}}

// OnVoicePause calls fn each time voice input must stop, until the returned
// func runs. fn may run on any thread.
func OnVoicePause(fn func()) func() {
	voicePause.Lock()
	defer voicePause.Unlock()
	id := voicePause.next
	voicePause.next++
	voicePause.listeners[id] = fn
	return func() {
		voicePause.Lock()
		defer voicePause.Unlock()
		delete(voicePause.listeners, id)
	}
}

// PauseVoiceInput tells every listener to stop voice input.
func PauseVoiceInput() {
	voicePause.Lock()
	listeners := make([]func(), 0, len(voicePause.listeners))
	for _, fn := range voicePause.listeners {
		listeners = append(listeners, fn)
	}
	voicePause.Unlock()
	for _, fn := range listeners {
		fn()
	}
}
