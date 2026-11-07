//go:build windows

package mcpapp

// Attach only to this App's WebView2. Interface identities/vtable indices are
// from Microsoft.Web.WebView2 1.0.3800.47's official WebView2.h (the same SDK
// already recorded by WinOne), not guest messages or reflection into MyGo.
import (
	"errors"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
	"golang.org/x/sys/windows"
)

var iidWebView4 = mustGUID("20d02d59-6df2-42dc-bd06-f98a694b1302")
var iidWebView22 = mustGUID("db75dfc7-a857-4632-a398-6969dde26c0a")
var iidFrame2 = mustGUID("7a6a5834-d185-4dbf-b63f-4a9bc43107d4")
var iidFrame7 = mustGUID("3598cfa2-d85d-5a9f-9228-4dde1f59ec64")
var iidFrameCreated = mustGUID("38059770-9baa-11eb-a8b3-0242ac130003")
var iidChildCreated = mustGUID("569e40e7-46b7-563d-83ae-1073155664d7")
var iidFrameNavigate = mustGUID("e79908bf-2d5d-4968-83db-263fea2c1da3")
var iidFrameDestroyed = mustGUID("59dd7b4c-9baa-11eb-a8b3-0242ac130003")
var iidAnyNavigate = mustGUID("9adbe429-f36d-432b-9ddc-f8881fbd76e3")
var iidUnknown = mustGUID("00000000-0000-0000-c000-000000000046")
var freeCOM = windows.NewLazySystemDLL("ole32.dll").NewProc("CoTaskMemFree")

func mustGUID(value string) windows.GUID {
	guid, err := windows.GUIDFromString("{" + value + "}")
	if err != nil {
		panic(err)
	}
	return guid
}

// comObject is the native IUnknown header shared by the SDK interfaces used
// here. Keep both lpVtbl and interface outputs as pointers; only SyscallN's
// ABI boundary accepts their integer addresses. Derived vtables extend this
// three-slot prefix, so method offsets never round-trip through uintptr.
type comObject struct{ table *comVTable }
type comVTable struct{ queryInterface, addRef, release uintptr }
type comEventVTable struct {
	comVTable
	invoke uintptr
}

//go:uintptrescapes
func com(object *comObject, index int, args ...uintptr) uintptr {
	method := *(*uintptr)(unsafe.Add(unsafe.Pointer(object.table), index*int(unsafe.Sizeof(uintptr(0)))))
	call := append([]uintptr{uintptr(unsafe.Pointer(object))}, args...)
	result, _, _ := syscall.SyscallN(method, call...)
	runtime.KeepAlive(object)
	return result
}
func failedCOM(result uintptr) bool { return int32(result) < 0 }
func queryCOM(object *comObject, iid *windows.GUID) *comObject {
	var out *comObject
	if object == nil || failedCOM(com(object, 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))) {
		return nil
	}
	return out
}
func releaseCOM(object *comObject) {
	if object != nil {
		com(object, 2)
	}
}
func navigationURI(args *comObject) string {
	var text *uint16
	if args == nil || failedCOM(com(args, 3, uintptr(unsafe.Pointer(&text)))) || text == nil {
		return ""
	}
	result := windows.UTF16PtrToString(text)
	freeCOM.Call(uintptr(unsafe.Pointer(text)))
	return result
}

type comHandler struct {
	comObject
	refs   atomic.Int32
	iid    windows.GUID
	invoke func(*comObject, *comObject)
}

var handlerTable comEventVTable
var handlers = struct {
	sync.Mutex
	all map[*comHandler]*comHandler
}{all: map[*comHandler]*comHandler{}}

func init() {
	handlerTable = comEventVTable{
		comVTable: comVTable{
			queryInterface: syscall.NewCallback(func(self *comHandler, iid *windows.GUID, out **comObject) uintptr {
				if out == nil {
					return 0x80004003 // E_POINTER; COM requires a writable output.
				}
				*out = nil
				handlers.Lock()
				handler := handlers.all[self]
				handlers.Unlock()
				if handler == nil || iid == nil || *iid != iidUnknown && *iid != handler.iid {
					return 0x80004002
				}
				*out = &handler.comObject
				handler.refs.Add(1)
				return 0
			}),
			addRef:  syscall.NewCallback(func(self *comHandler) uintptr { return handlerRef(self, 1) }),
			release: syscall.NewCallback(func(self *comHandler) uintptr { return handlerRef(self, -1) }),
		},
		invoke: syscall.NewCallback(func(self *comHandler, sender, args *comObject) uintptr {
			handlers.Lock()
			handler := handlers.all[self]
			handlers.Unlock()
			if handler != nil {
				handler.invoke(sender, args)
			}
			return 0
		}),
	}
}
func handlerRef(self *comHandler, delta int32) uintptr {
	handlers.Lock()
	defer handlers.Unlock()
	handler := handlers.all[self]
	if handler == nil {
		return 0
	}
	count := handler.refs.Add(delta)
	if count == 0 {
		delete(handlers.all, self)
	}
	return uintptr(count)
}
func addCOMEvent(object *comObject, index int, iid windows.GUID, fn func(*comObject, *comObject)) (int64, error) {
	handler := &comHandler{comObject: comObject{table: &handlerTable.comVTable}, iid: iid, invoke: fn}
	handler.refs.Store(1)
	handlers.Lock()
	handlers.all[handler] = handler
	handlers.Unlock()
	var token int64
	result := com(object, index, uintptr(unsafe.Pointer(&handler.comObject)), uintptr(unsafe.Pointer(&token)))
	handlerRef(handler, -1)
	if failedCOM(result) {
		return 0, errors.New("MCP_APP_NATIVE_POLICY_UNAVAILABLE")
	}
	return token, nil
}

type nativeFrame struct {
	frame2, frame7               *comObject
	nav, child, destroy          int64
	root, rootLoaded, closed     bool
	hasNav, hasChild, hasDestroy bool
	address                      string
}
type nativePolicy struct {
	webview, resources        *comObject
	filters                   []string
	session                   Session
	created, anyNavigation    int64
	frames                    map[*nativeFrame]bool
	closed, failed            bool
	hasCreated, hasNavigation bool
	onFailure                 func(error)
}

func (p *nativePolicy) fail() {
	if p.failed || p.closed {
		return
	}
	p.failed = true
	p.onFailure(errors.New("MCP_APP_NATIVE_POLICY_UNAVAILABLE"))
}
func installFramePolicy(view *mygo.WebView, session Session, onFailure func(error)) (framePolicy, error) {
	native, supported := any(view.Page()).(interface{ QueryNativeWebViewInterface(string) unsafe.Pointer })
	if !supported {
		return nil, errors.New("MCP_APP_NATIVE_POLICY_UNAVAILABLE")
	}
	// QueryInterface writes actual interface pointers and transfers one native
	// reference. The old integer handle is never reinterpreted as a pointer.
	webview := (*comObject)(native.QueryNativeWebViewInterface(strings.Trim(iidWebView4.String(), "{}")))
	if webview == nil {
		return nil, errors.New("MCP_APP_NATIVE_POLICY_UNAVAILABLE")
	}
	resources := (*comObject)(native.QueryNativeWebViewInterface(strings.Trim(iidWebView22.String(), "{}")))
	if resources == nil {
		releaseCOM(webview)
		return nil, errors.New("MCP_APP_NATIVE_POLICY_UNAVAILABLE")
	}
	policy := &nativePolicy{webview: webview, resources: resources, session: session, frames: map[*nativeFrame]bool{}, onFailure: onFailure}
	var err error
	policy.anyNavigation, err = addCOMEvent(webview, 17, iidAnyNavigate, func(_, args *comObject) {
		// A Runtime missing Frame2/Frame7 must not run the first App document
		// between capability discovery and the native close callback.
		if policy.failed || policy.closed {
			com(args, 8, 1)
		}
	})
	if err == nil {
		policy.hasNavigation = true
		policy.created, err = addCOMEvent(webview, 73, iidFrameCreated, func(_, args *comObject) { policy.createdFrame(args, true) })
		policy.hasCreated = err == nil
	}
	if err == nil {
		err = policy.addDocumentFilters()
	}
	if err != nil {
		policy.close()
		return nil, err
	}
	return policy, nil
}
func (p *nativePolicy) addDocumentFilters() error {
	// MyGo uses the legacy WebResourceRequested filter. Microsoft
	// documents that _22 is required for different-origin descendant frames;
	// without it a permitted local iframe falls through to system networking.
	// Apply only document sources, never service/shared workers or another view.
	patterns := []string{browserOrigin(p.session.ViewID, false) + "/*", browserOrigin(p.session.ViewID, true) + "/*"}
	for _, domain := range p.session.Policy.FrameDomains {
		u, err := url.Parse(domain)
		if err != nil || !strings.HasSuffix(u.Hostname(), ".localhost") {
			continue
		}
		labels := strings.Split(strings.TrimSuffix(u.Hostname(), ".localhost"), ".")
		if mygo.Protocol.IsHandled(labels[len(labels)-1]) {
			patterns = append(patterns, strings.TrimSuffix(domain, "/")+"/*")
		}
	}
	for _, pattern := range patterns {
		uri, err := windows.UTF16PtrFromString(pattern)
		if err != nil || failedCOM(com(p.resources, 123, uintptr(unsafe.Pointer(uri)), 0, 1)) {
			return errors.New("MCP_APP_NATIVE_POLICY_UNAVAILABLE")
		}
		p.filters = append(p.filters, pattern)
	}
	return nil
}
func (p *nativePolicy) createdFrame(args *comObject, root bool) {
	if p.closed {
		return
	}
	var raw *comObject
	if failedCOM(com(args, 3, uintptr(unsafe.Pointer(&raw)))) || raw == nil {
		p.fail()
		return
	}
	defer releaseCOM(raw)
	frame := &nativeFrame{frame2: queryCOM(raw, &iidFrame2), frame7: queryCOM(raw, &iidFrame7), root: root}
	if frame.frame2 == nil || frame.frame7 == nil {
		releaseCOM(frame.frame2)
		releaseCOM(frame.frame7)
		p.fail()
		return
	}
	p.frames[frame] = true
	var err error
	frame.nav, err = addCOMEvent(frame.frame2, 11, iidFrameNavigate, func(_, args *comObject) {
		allowed := !p.closed && !p.failed && !frame.closed
		address := navigationURI(args)
		if root {
			allowed = allowed && (sameResourceDocument(p.session.ViewID, address) || address == "about:blank" && !frame.rootLoaded)
			if sameResourceDocument(p.session.ViewID, address) {
				frame.rootLoaded = true
			}
		} else {
			allowed = allowed && frameURLAllowed(address, p.session.Policy.FrameDomains)
		}
		if !allowed {
			com(args, 8, 1)
			if root && !p.closed && !p.failed {
				// A native bypass reaches this final gate. WebView2 may discard
				// the old document while cancelling; revoke instead of replaying
				// App HTML or keeping a live bridge on its error document.
				p.failed = true
				p.onFailure(errors.New("MCP_APP_DOCUMENT_NAVIGATION_BLOCKED"))
			}
		} else {
			frame.address = address
		}
	})
	if err == nil {
		frame.hasNav = true
		frame.child, err = addCOMEvent(frame.frame7, 30, iidChildCreated, func(_, args *comObject) { p.createdFrame(args, false) })
		frame.hasChild = err == nil
	}
	if err == nil {
		frame.destroy, err = addCOMEvent(frame.frame2, 8, iidFrameDestroyed, func(_, _ *comObject) { p.dropFrame(frame, true) })
		frame.hasDestroy = err == nil
	}
	if err != nil {
		p.dropFrame(frame, false)
		p.fail()
	}
}
func (p *nativePolicy) dropFrame(frame *nativeFrame, destroyed bool) {
	if frame.closed {
		return
	}
	frame.closed = true
	delete(p.frames, frame)
	// Destroyed frames permit only IsDestroyed/IUnknown operations. The Runtime
	// retires their subscriptions itself; removing Frame7 children here can
	// hit a native invariant during recursive destruction. Live normal close
	// still removes every registration before releasing our COM references.
	if !destroyed {
		var gone int32
		com(frame.frame2, 10, uintptr(unsafe.Pointer(&gone)))
		destroyed = gone != 0
	}
	if !destroyed {
		if frame.hasNav {
			com(frame.frame2, 12, uintptr(frame.nav))
		}
		if frame.hasChild {
			com(frame.frame7, 31, uintptr(frame.child))
		}
		if frame.hasDestroy {
			com(frame.frame2, 9, uintptr(frame.destroy))
		}
	}
	releaseCOM(frame.frame2)
	releaseCOM(frame.frame7)
}
func (p *nativePolicy) close() {
	if p.closed {
		return
	}
	p.closed = true
	if p.hasNavigation {
		com(p.webview, 18, uintptr(p.anyNavigation))
	}
	if p.hasCreated {
		com(p.webview, 74, uintptr(p.created))
	}
	for _, pattern := range p.filters {
		uri, _ := windows.UTF16PtrFromString(pattern)
		com(p.resources, 124, uintptr(unsafe.Pointer(uri)), 0, 1)
	}
	releaseCOM(p.resources)
	for frame := range p.frames {
		p.dropFrame(frame, false)
	}
	releaseCOM(p.webview)
}
func sameResourceDocument(id, address string) bool {
	parsed, err := url.Parse(address)
	if err != nil || parsed.User != nil || parsed.Path != "/resource" || parsed.RawQuery != "" {
		return false
	}
	return parsed.Scheme == "http" && parsed.Host == id+"-view.cindy-mcp-app.localhost" || parsed.Scheme == "cindy-mcp-app" && parsed.Host == id+"-view"
}
func defaultPort(address *url.URL) string {
	port := address.Port()
	if address.Scheme == "http" && port == "80" || address.Scheme == "https" && port == "443" {
		return ""
	}
	return port
}

// Native equivalent of Cindy's shared mcpAppFrameUrlAllowed, applied to actual
// descendant frame navigation only; a root App never becomes an external page.
func frameURLAllowed(address string, domains []string) bool {
	if address == "about:blank" || address == "about:srcdoc" {
		return true
	}
	target, err := url.Parse(address)
	if err != nil || target.User != nil {
		return false
	}
	target.Scheme = strings.ToLower(target.Scheme)
	if target.Scheme != "http" && target.Scheme != "https" {
		return false
	}
	for _, domain := range domains {
		wildcard := strings.Contains(domain, "://*.")
		anyPort := strings.HasSuffix(strings.TrimSuffix(domain, "/"), ":*")
		cleaned := strings.Replace(domain, "://*.", "://", 1)
		if anyPort {
			cleaned = strings.TrimSuffix(strings.TrimSuffix(cleaned, "/"), ":*")
		}
		source, err := url.Parse(cleaned)
		if err != nil {
			continue
		}
		host := strings.ToLower(target.Hostname())
		expected := strings.ToLower(source.Hostname())
		if target.Scheme == source.Scheme && (anyPort || defaultPort(source) == defaultPort(target)) &&
			(wildcard && strings.HasSuffix(host, "."+expected) || !wildcard && host == expected) {
			return true
		}
	}
	return false
}
