//go:build windows

package mcpapp

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func liveCOMHandlers() int {
	handlers.Lock()
	defer handlers.Unlock()
	return len(handlers.all)
}

// These tests cross the real Windows callback/SyscallN ABI. They do not replace
// the existing WebView2 security tests with a simulated browser or App runtime.
func TestNativeCOMHandlerQueryInterfaceIdentityAndLifetime(t *testing.T) {
	before := liveCOMHandlers()
	var gotSender, gotArgs *comObject
	handler := &comHandler{comObject: comObject{table: &handlerTable.comVTable}, iid: iidFrameCreated,
		invoke: func(sender, args *comObject) { gotSender, gotArgs = sender, args }}
	handler.refs.Store(1)
	handlers.Lock()
	handlers.all[handler] = handler
	handlers.Unlock()
	t.Cleanup(func() {
		for remaining := handler.refs.Load(); remaining > 0; remaining-- {
			releaseCOM(&handler.comObject)
		}
		if got := liveCOMHandlers(); got != before {
			t.Errorf("COM handler retained after final native Release: %d want %d", got, before)
		}
	})
	object := &handler.comObject
	if com(object, 1) != 2 || com(object, 2) != 1 {
		t.Fatal("native AddRef/Release did not preserve the owning reference")
	}
	for _, iid := range []*windows.GUID{&iidUnknown, &iidFrameCreated} {
		queried := queryCOM(object, iid)
		if queried != object || handler.refs.Load() != 2 {
			t.Fatal("QueryInterface lost its typed identity or owning reference")
		}
		releaseCOM(queried)
	}
	for _, value := range []string{
		"00000003-0000-0000-c000-000000000046", // IMarshal
		"94ea2b94-e9cc-49e0-c0ff-ee64ca8f5b90", // IAgileObject
		"ecc8691b-c1db-4dc0-855e-65f6c551af49", // INoMarshal
		"00000001-0000-0000-c000-000000000046", // unrelated interface
	} {
		iid := mustGUID(value)
		out := object
		status := com(object, 0, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&out)))
		if uint32(status) != 0x80004002 || out != nil || handler.refs.Load() != 1 {
			t.Fatalf("unsupported interface %s retained an output/ref: %#x %p %d", value, status, out, handler.refs.Load())
		}
	}
	if queryCOM(nil, &iidUnknown) != nil || queryCOM(object, nil) != nil {
		t.Fatal("a missing object/IID produced an interface")
	}
	if status := com(object, 0, uintptr(unsafe.Pointer(&iidUnknown)), 0); uint32(status) != 0x80004003 {
		t.Fatalf("missing QueryInterface output returned %#x instead of E_POINTER", status)
	}
	sender, args := &comObject{}, &comObject{}
	runtime.GC()
	if status := com(object, 3, uintptr(unsafe.Pointer(sender)), uintptr(unsafe.Pointer(args))); failedCOM(status) {
		t.Fatalf("native typed Invoke failed: %#x", status)
	}
	if gotSender != sender || gotArgs != args {
		t.Fatal("native Invoke changed its sender/event-argument pointers")
	}
	releaseCOM(object)
	if handler.refs.Load() != 0 || liveCOMHandlers() != before {
		t.Fatal("last native Release did not remove the handler root")
	}
}

type nativeCOMEventSource struct {
	comObject
	held  *comHandler
	token int64
	fail  bool
}

type nativeCOMEventSourceTable struct {
	comVTable
	addEvent, removeEvent uintptr
}

func TestNativeCOMEventRegistrationTypedTokenAndFailureCleanup(t *testing.T) {
	before := liveCOMHandlers()
	table := &nativeCOMEventSourceTable{
		addEvent: syscall.NewCallback(func(source *nativeCOMEventSource, handler *comHandler, token *int64) uintptr {
			if source.fail {
				return 0x80070005
			}
			com(&handler.comObject, 1)
			source.held = handler
			*token = source.token
			return 0
		}),
		removeEvent: syscall.NewCallback(func(source *nativeCOMEventSource, token int64) uintptr {
			if source.held == nil || token != source.token {
				return 0x80070057
			}
			releaseCOM(&source.held.comObject)
			source.held = nil
			return 0
		}),
	}
	source := &nativeCOMEventSource{comObject: comObject{table: &table.comVTable}, token: -0x123456789abcdef}
	t.Cleanup(func() {
		if source.held != nil {
			releaseCOM(&source.held.comObject)
		}
		if got := liveCOMHandlers(); got != before {
			t.Errorf("event registration leaked native callback roots: %d want %d", got, before)
		}
	})
	var gotSender, gotArgs *comObject
	token, err := addCOMEvent(&source.comObject, 3, iidFrameCreated, func(sender, args *comObject) { gotSender, gotArgs = sender, args })
	if err != nil || token != source.token || source.held == nil || source.held.refs.Load() != 1 {
		t.Fatalf("native subscription did not retain exactly its own ref/token: %d %v", token, err)
	}
	held := source.held
	sender, args := &comObject{}, &comObject{}
	runtime.GC()
	com(&held.comObject, 3, uintptr(unsafe.Pointer(sender)), uintptr(unsafe.Pointer(args)))
	if gotSender != sender || gotArgs != args {
		t.Fatal("registered native callback lost its typed arguments")
	}
	if status := com(&source.comObject, 4, uintptr(token)); failedCOM(status) {
		t.Fatalf("native unsubscription lost its 64-bit token: %#x", status)
	}
	if held.refs.Load() != 0 || liveCOMHandlers() != before {
		t.Fatal("native unsubscription did not reclaim the callback")
	}
	source.fail = true
	token, err = addCOMEvent(&source.comObject, 3, iidFrameCreated, func(*comObject, *comObject) {})
	if token != 0 || err == nil || err.Error() != "MCP_APP_NATIVE_POLICY_UNAVAILABLE" || liveCOMHandlers() != before {
		t.Fatalf("failed subscription retained a token/callback or lost its failure: %d %v", token, err)
	}
}

func TestNativeCOMScriptCompletionKeepsHRESULTAndUTF16Types(t *testing.T) {
	before := liveCOMHandlers()
	var gotStatus int32
	var gotText *uint16
	completed := newNativeScriptCompletion(func(status int32, text *uint16) { gotStatus, gotText = status, text })
	t.Cleanup(func() {
		for remaining := completed.refs.Load(); remaining > 0; remaining-- {
			releaseCOM(&completed.comObject)
		}
	})
	text, err := windows.UTF16PtrFromString(`"实际 App 回执"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []uint32{0, 0x80004005} {
		runtime.GC()
		if result := com(&completed.comObject, 3, uintptr(status), uintptr(unsafe.Pointer(text))); failedCOM(result) {
			t.Fatalf("native completion Invoke failed: %#x", result)
		}
		if gotStatus != int32(status) || gotText != text || windows.UTF16PtrToString(gotText) != `"实际 App 回执"` {
			t.Fatal("completion reinterpreted its HRESULT or LPCWSTR as an interface pointer")
		}
	}
	queried := queryCOM(&completed.comObject, &completed.iid)
	if queried != &completed.comObject {
		t.Fatal("completion QueryInterface lost its native identity")
	}
	releaseCOM(queried)
	releaseCOM(&completed.comObject)
	if completed.refs.Load() != 0 || liveCOMHandlers() != before {
		t.Fatal("completion retained a native callback after final Release")
	}
}
