//go:build windows && (amd64 || arm64)

package windows

// This registry is main-thread-only, like WebView2 and the native input router.
// It contains callbacks, not credentials, message content or a guest bridge.
var winOneWebViewInput = map[*window]map[uint64]func(){}
var winOneWebViewInputSequence uint64

func (w *window) AddUserGestureListener(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	winOneWebViewInputSequence++
	id := winOneWebViewInputSequence
	listeners := winOneWebViewInput[w]
	if listeners == nil {
		listeners = map[uint64]func(){}
		winOneWebViewInput[w] = listeners
	}
	listeners[id] = fn
	return func() {
		delete(listeners, id)
		if len(listeners) == 0 {
			delete(winOneWebViewInput, w)
		}
	}
}

// Only OS-dispatched input sites call this, after native hit testing or a
// WebView2 accelerator event. The document cannot post a message to mint it.
func (w *window) winOneRecordUserGesture() {
	if w.closed || w.host == nil || !w.host.IsFocused() {
		return
	}
	for _, callback := range winOneWebViewInput[w] {
		callback()
	}
}
