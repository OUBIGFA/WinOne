package mygo

import "github.com/egoist/mygo/internal/platform"

// IsFocused reports the embedded page's native keyboard focus, not its host
// window's focus or a page-reported document.hasFocus(). WinOne's plugin-link
// gate uses it to distinguish a real focused guest from a background page.
func (v *WebView) IsFocused() bool {
	return get(v.w, func(native platform.Window) bool {
		embedded, ok := native.(*webViewNative)
		if !ok || !embedded.shown || !embedded.Window.IsFocused() {
			return false
		}
		focused, ok := embedded.web.(interface{ IsFocused() bool })
		return ok && focused.IsFocused()
	})
}
