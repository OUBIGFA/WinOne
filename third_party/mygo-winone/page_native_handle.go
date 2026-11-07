package mygo

import (
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// NativeWebViewHandle gives trusted native content hosts the backend's actual
// WebView object so frame/request policy is attached to that object, never to a
// guest-supplied URL or handle. Plugin pages themselves expose no Go bindings.
func (p *Page) NativeWebViewHandle() uintptr {
	return pageGet(p.w, platform.Window.WebViewHandle)
}

// QueryNativeWebViewInterface acquires one owned COM interface from this exact
// native WebView. Only trusted Go hosts can use it; the caller must Release the
// result on the UI thread. Unsupported or no-longer-live pages return nil.
// A typed QueryInterface out parameter supplies the pointer, never a restored
// uintptr address, so native security policy keeps a real COM reference.
func (p *Page) QueryNativeWebViewInterface(iid string) unsafe.Pointer {
	return pageGet(p.w, func(native platform.Window) unsafe.Pointer {
		var query any = native
		if embedded, ok := native.(*webViewNative); ok {
			if !embedded.shown {
				return nil
			}
			query = embedded.web
		}
		if source, ok := query.(interface{ QueryNativeWebViewInterface(string) unsafe.Pointer }); ok {
			return source.QueryNativeWebViewInterface(iid)
		}
		return nil
	})
}

// OnUserGesture reports a real pointer/keyboard event received by the embedded
// native browser. DOM dispatch, a focus change and a rendered frame do not call
// it. Unsupported backends cannot manufacture a browser gesture.
func (v *WebView) OnUserGesture(fn func()) func() {
	var remove func()
	get(v.w, func(native platform.Window) bool {
		embedded, ok := native.(*webViewNative)
		if !ok {
			return false
		}
		input, ok := embedded.web.(interface{ AddUserGestureListener(func()) func() })
		if !ok {
			return false
		}
		remove = input.AddUserGestureListener(fn)
		return true
	})
	return func() {
		v.w.do(func(platform.Window) {
			if remove != nil {
				remove()
				remove = nil
			}
		})
	}
}
