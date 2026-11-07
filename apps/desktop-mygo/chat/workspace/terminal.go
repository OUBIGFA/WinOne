package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// From third_party/native-terminal.json / MyGo v0.7.4 mygo-plugin.json.
// build/verify-native-terminal.mjs checks this binding as well as the DLL.
const terminalLibrarySHA256 = "53e8e2dd979702e8f2c48107af19be5a76efc2e88eb7cf1097218e899665da41"

var terminalLibraryMu sync.Mutex

func loadTerminalLibrary(path string) error {
	if path == "" {
		path = os.Getenv("WINONE_TERMINAL_LIBRARY")
	}
	if path == "" {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		path = filepath.Join(filepath.Dir(executable), "ghostty-vt.dll")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	file, err := os.Open(absolute)
	if err != nil {
		return errors.New(wt("terminalLibraryMissing"))
	}
	hash := sha256.New()
	_, readErr := io.Copy(hash, file)
	closeErr := file.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if hex.EncodeToString(hash.Sum(nil)) != terminalLibrarySHA256 {
		return errors.New(wt("terminalLibraryMismatch"))
	}
	terminalLibraryMu.Lock()
	defer terminalLibraryMu.Unlock()
	// This process-only setting is explicit and verified. In particular, a
	// missing bundled file never reaches MyGo's user-cache download fallback.
	if err = os.Setenv("MYGO_GHOSTTY_VT", absolute); err != nil {
		return err
	}
	return terminal.Load()
}

type TerminalOptions struct {
	Prefix, SessionID, TabID string
	LibraryPath              string
	OnRunning                func(bool)
	OnLink                   func(string)
}
type TerminalPane struct {
	env                                      *core.Env
	options                                  TerminalOptions
	term                                     *terminal.Terminal
	conn                                     *terminalConnection
	info                                     terminalInfo
	shell                                    string
	loading, running, closed, visible, focus bool
	generation                               uint64
	err                                      string
	exit                                     *int
}

func NewTerminalPane(env *core.Env, options TerminalOptions) *TerminalPane {
	p := &TerminalPane{env: env, options: options, shell: "powershell", visible: true, focus: true}
	p.Start()
	return p
}
func (p *TerminalPane) Running() bool { return p.running }
func (p *TerminalPane) Busy() bool    { return p.loading }
func (p *TerminalPane) Start() {
	if p.loading || p.closed || p.options.SessionID == "" {
		return
	}
	p.loading = true
	p.generation++
	generation := p.generation
	p.err = ""
	p.exit = nil
	previous := p.term
	p.term = nil
	p.conn = nil
	shell := p.shell
	type opened struct {
		term *terminal.Terminal
		conn *terminalConnection
		info terminalInfo
	}
	core.Go(p.env, func() (opened, error) {
		var result opened
		if previous != nil {
			if err := previous.Close(); err != nil {
				return result, err
			}
		}
		if err := loadTerminalLibrary(p.options.LibraryPath); err != nil {
			return result, err
		}
		onError := func(err error) {
			p.env.Post(func() {
				if !p.closed && generation == p.generation {
					p.err = err.Error()
				}
			})
		}
		onExit := func(code int) {
			p.env.Post(func() {
				if !p.closed && generation == p.generation {
					p.exit = &code
					p.running = false
					if p.options.OnRunning != nil {
						p.options.OnRunning(false)
					}
				}
			})
		}
		conn, info, err := newTerminalConnection(p.env, p.options.Prefix, p.options.SessionID, p.options.TabID, shell, onError, onExit)
		if err != nil {
			return result, err
		}
		term, err := terminal.New(terminal.Options{Conn: conn, Font: terminal.Font{Family: kit.MonoFont, Size: 13}, OpenLink: func(url string) bool {
			if p.options.OnLink != nil {
				p.options.OnLink(url)
			} else {
				core.Do(p.env, func() error {
					return p.env.Host.CallInto(p.options.Prefix+"link.open", map[string]string{"url": url}, nil)
				}, func(err error) {
					if err != nil {
						onError(err)
					}
				})
			}
			return true
		}})
		if err != nil {
			_ = conn.Close()
			return result, err
		}
		return opened{term, conn, info}, nil
	}, func(result opened, err error) {
		if p.closed || generation != p.generation {
			if result.term != nil {
				core.Do(p.env, result.term.Close, func(error) {})
			}
			return
		}
		p.loading = false
		if err != nil {
			p.err = err.Error()
			p.running = false
			if p.options.OnRunning != nil {
				p.options.OnRunning(false)
			}
			return
		}
		p.term, p.conn, p.info = result.term, result.conn, result.info
		p.conn.visible.Store(p.visible)
		p.running = p.exit == nil
		select {
		case <-p.term.Done():
			p.running = false
		default:
		}
		p.focus = true
		if p.options.OnRunning != nil {
			p.options.OnRunning(p.running)
		}
	})
}
func (p *TerminalPane) SetVisible(visible bool) {
	if p.closed || p.visible == visible {
		return
	}
	p.visible = visible
	if p.conn != nil {
		p.conn.visible.Store(visible)
	}
	if visible {
		p.focus = true
	}
}
func (p *TerminalPane) Close() {
	if p.closed {
		return
	}
	p.closed = true
	p.generation++
	current := p.term
	p.term = nil
	p.conn = nil
	if current != nil {
		core.Do(p.env, current.Close, func(err error) {
			if err != nil && p.env.Toast != nil {
				p.env.Toast(err.Error())
			}
		})
	}
	if p.options.OnRunning != nil {
		p.options.OnRunning(false)
	}
}
func (p *TerminalPane) View(c *ui.Context) {
	if p.closed || !p.visible {
		return
	}
	ui.Column(c).Fill().MinWidth(0).Gap(6).Children(func() {
		ui.Row(c).FillWidth().Gap(6).Padding(8).Wrap().Children(func() {
			shell := p.shell
			changed, trigger := kit.Select(c, &shell, []kit.SelectOption{{Value: "powershell", Label: "Windows PowerShell"}, {Value: "pwsh", Label: "PowerShell 7"}, {Value: "cmd", Label: "Command Prompt"}})
			trigger.Disabled(p.loading)
			if changed && !p.loading {
				p.shell = shell
				p.Start()
			}
			if kit.IconButton(c, icons.RotateCcw, i18n.T("chat.terminalRestart")).Disabled(p.loading).Clicked() {
				p.Start()
			}
			if kit.IconButton(c, icons.Eraser, i18n.T("chat.terminalClear")).Disabled(p.term == nil).Clicked() {
				p.term.ClearScreen()
			}
		})
		kit.InlineError(c, p.err)
		if p.loading {
			kit.Loader(c)
		}
		if p.options.SessionID == "" {
			kit.Help(c, i18n.T("chat.selectConversation"))
		}
		if p.exit != nil {
			kit.Help(c, wt("terminalExited", "code", *p.exit))
		}
		if p.term != nil {
			view := terminal.View(c, p.term).Grow(1).FillWidth().MinHeight(0).Label(i18n.T("chat.terminal"))
			if p.focus {
				view.Focus()
				p.focus = false
			}
		}
	})
}
