package workspace

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

type terminalFixture struct {
	mu                sync.Mutex
	chunks            [][]byte
	offset            uint64
	writes            [][]byte
	sizes             [][2]int
	disposed, created int
	end, failWrite    bool
}

func terminalHost(t *testing.T, f *terminalFixture) *testhost.Harness {
	return testhost.New(t, func(method string, raw json.RawMessage) (any, error) {
		var input struct {
			SessionID, TabID, Shell string
			DataBase64              string
			Cols, Rows              int
			Offset                  uint64
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		if input.SessionID != "session" || input.TabID != "terminal-one" {
			return nil, fmt.Errorf("wrong terminal scope")
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch method {
		case "chat.terminal.create":
			f.created++
			return terminalInfo{ID: input.TabID, Shell: input.Shell, Directory: `C:\owned`}, nil
		case "chat.terminal.poll":
			result := terminalPoll{Offset: f.offset}
			if len(f.chunks) > 0 {
				result.Data = f.chunks[0]
				f.chunks = f.chunks[1:]
				f.offset += uint64(len(result.Data))
				result.Offset = f.offset
			} else if f.end {
				code := uint32(0)
				result.Exit = &code
			}
			return result, nil
		case "chat.terminal.write":
			data, err := base64.StdEncoding.DecodeString(input.DataBase64)
			if err != nil {
				return nil, err
			}
			f.writes = append(f.writes, data)
			if f.failWrite {
				return nil, fmt.Errorf("AMBIGUOUS_WRITE_FAILURE")
			}
			return nil, nil
		case "chat.terminal.resize":
			f.sizes = append(f.sizes, [2]int{input.Cols, input.Rows})
			return nil, nil
		case "chat.terminal.dispose":
			f.disposed++
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected %s", method)
		}
	})
}
func TestTerminalConnectionPreservesOutputAndInputBytes(t *testing.T) {
	f := &terminalFixture{chunks: [][]byte{{0xe4, 0xb8}, {0xad, '!', 0x1b, '[', '3', '1', 'm', 'R'}}, end: true}
	h := terminalHost(t, f)
	exits := 0
	c, _, err := newTerminalConnection(h.Env, "chat.", "session", "terminal-one", "cmd", nil, func(code int) {
		exits++
		if code != 0 {
			t.Errorf("exit code=%d", code)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	data, err := io.ReadAll(c)
	if err != nil || !bytes.Equal(data, []byte("中!\x1b[31mR")) {
		t.Fatalf("VT stream changed: %x %v", data, err)
	}
	if exits != 1 {
		t.Fatalf("terminal exit callbacks=%d", exits)
	}
	input := []byte{0xe4, 0xb8, 0, 0xff}
	if _, err = c.Write(input); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	got := append([]byte(nil), f.writes[0]...)
	f.mu.Unlock()
	if !bytes.Equal(input, got) {
		t.Fatalf("VT input changed: %x", got)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.disposed != 1 {
		t.Fatalf("terminal was disposed %d times", f.disposed)
	}
}
func TestFailedTerminalWriteIsNeverReplayed(t *testing.T) {
	f := &terminalFixture{failWrite: true}
	h := terminalHost(t, f)
	errors := 0
	c, _, err := newTerminalConnection(h.Env, "chat.", "session", "terminal-one", "cmd", func(error) { errors++ }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.Write([]byte("first")); err == nil {
		t.Fatal("ambiguous write reported success")
	}
	if _, err = c.Write([]byte("next")); err == nil {
		t.Fatal("failed stream accepted more input")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) != 1 || errors != 1 {
		t.Fatalf("write replay/errors: %d / %d", len(f.writes), errors)
	}
}
func TestMissingTerminalLibraryDoesNotStartAShellOrDownload(t *testing.T) {
	f := &terminalFixture{}
	h := terminalHost(t, f)
	p := NewTerminalPane(h.Env, TerminalOptions{Prefix: "chat.", SessionID: "session", TabID: "terminal-one", LibraryPath: filepath.Join(t.TempDir(), "not-present.dll")})
	t.Cleanup(p.Close)
	tt := ui.NewTester(func(c *ui.Context) { p.View(c) }, 600, 400)
	for range 3 {
		h.Settle(tt)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.created != 0 || p.err == "" {
		t.Fatalf("missing library started a process or hid failure: created=%d error=%s", f.created, p.err)
	}
}
func TestOfficialNativeTerminalRendersVTAndSendsThroughHost(t *testing.T) {
	library := os.Getenv("WINONE_TERMINAL_LIBRARY")
	if library == "" {
		t.Skip("explicit verified native terminal library is required")
	}
	f := &terminalFixture{chunks: [][]byte{{'R', 'e', 'a', 'd', 'y', ' ', 0xe4, 0xb8}, {0xad, '\r', '\n'}, []byte("\x1b[31mRED\x1b[0m\r\n")}}
	h := terminalHost(t, f)
	p := NewTerminalPane(h.Env, TerminalOptions{Prefix: "chat.", SessionID: "session", TabID: "terminal-one", LibraryPath: library})
	t.Cleanup(p.Close)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); p.View(c) }, 700, 450)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h.Settle(tt)
		if p.term != nil && strings.Contains(p.term.Text(), "RED") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if p.term == nil || p.err != "" || !strings.Contains(p.term.Text(), "Ready 中") {
		t.Fatalf("native VT did not render: %s", p.err)
	}
	tt.Type("echo hello")
	tt.Key(0, ui.KeyEnter)
	delivered := false
	for time.Now().Before(deadline) {
		h.Settle(tt)
		f.mu.Lock()
		text := string(bytes.Join(f.writes, nil))
		f.mu.Unlock()
		if strings.Contains(text, "echo hello") {
			delivered = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !delivered {
		t.Fatal("native terminal input did not reach the Host")
	}
	p.Close()
	for range 3 {
		h.Settle(tt)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.created != 1 || f.disposed != 1 {
		t.Fatalf("native terminal lifetime: %d creates, %d disposes", f.created, f.disposed)
	}
}
