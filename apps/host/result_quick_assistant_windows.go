//go:build windows

package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"unicode/utf8"
	"winone/internal/resultwindow"
)

func (s *resultWindows) allowQuickAssistant(allowed bool) {
	s.mu.Lock()
	s.quickAssistantAllowed = allowed
	s.mu.Unlock()
}

// Repeated invocation activates the same conversation instead of discarding
// drafts or starting additional result/AI processes.
func (s *resultWindows) openQuickAssistant(actions []resultwindow.SelectionAction) error {
	return s.openQuickAssistantWithText("", actions)
}
func (s *resultWindows) openQuickAssistantWithText(text string, actions []resultwindow.SelectionAction, options ...resultwindow.QuickOptions) error {
	var quick resultwindow.QuickOptions
	if len(options) > 0 {
		quick = options[0]
	}
	s.quickShow.Lock()
	defer s.quickShow.Unlock()
	s.mu.Lock()
	if !s.quickAssistantAllowed {
		s.mu.Unlock()
		return errors.New("快捷助手已关闭")
	}
	for _, entry := range s.sessions {
		if entry.source == "quick-assistant" && entry.attached && entry.process != nil {
			process, id := entry.process, entry.id
			s.mu.Unlock()
			return process.send(resultwindow.Command{Kind: "activate", ID: id, Initial: &resultwindow.Initial{Actions: actions, Text: text, Quick: quick}})
		}
	}
	s.mu.Unlock()
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	return s.openSource(hex.EncodeToString(id[:]), "chat", nil, text, [2]int32{}, uint32(os.Getpid()), actions, quick)
}

// Only the Host's attached, live child can be handed back to a foreground
// caller. A simultaneous module stop must not produce a successful show reply.
func (s *resultWindows) quickAssistantTarget() (resultwindow.WindowTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.quickAssistantAllowed {
		for _, entry := range s.sessions {
			if entry.source == "quick-assistant" && entry.attached && !entry.closing && entry.window != "" && entry.process != nil && entry.process.cmd != nil && entry.process.cmd.Process != nil {
				select {
				case <-entry.process.done:
					continue
				default:
				}
				return resultwindow.WindowTarget{Window: entry.window, ProcessID: uint32(entry.process.cmd.Process.Pid)}, nil
			}
		}
	}
	return resultwindow.WindowTarget{}, errors.New("快捷助手窗口已关闭，请重新打开")
}

// Only an attached selection window may provide the selected source text.
func (s *resultWindows) validateSelectionHandoff(input resultwindow.SelectionHandoff, pid uint32) error {
	if strings.TrimSpace(input.Text) == "" || len(input.Text) > 65536 || !utf8.ValidString(input.Text) {
		return errors.New("选中文字为空或过长，请重新选择")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.sessions[input.Session]
	if entry == nil || entry.source != "selection" || !entry.attached || entry.process == nil || entry.process.cmd == nil || entry.process.cmd.Process == nil || uint32(entry.process.cmd.Process.Pid) != pid {
		return errors.New("划词窗口已失效，请重新选择文字")
	}
	return nil
}
