//go:build windows && (amd64 || arm64)

package windows

import (
 "strings"
 "unsafe"

 system "golang.org/x/sys/windows"
)

// QueryNativeWebViewInterface is a main-thread, trusted-host-only acquisition
// of the current native object. COM writes an actual owned pointer to void**;
// it is never rebuilt from the integer WebViewHandle kept by the backend.
// The native caller must release the returned reference on the UI thread.
func (w *window) QueryNativeWebViewInterface(rawIID string) unsafe.Pointer {
 if w.closed || w.webview == 0 || w.surface != nil || !w.ready {return nil}
 iid,err:=system.GUIDFromString("{"+strings.Trim(rawIID,"{}")+"}")
 if err!=nil{return nil}
 var acquired unsafe.Pointer
 hr:=comCall(w.webview,0,uintptr(unsafe.Pointer(&iid)),uintptr(unsafe.Pointer(&acquired)))
 if int32(hr)<0{return nil}
 return acquired
}
