//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"winone/internal/ai"
)

// Only the enabled capture module using local recognition pays the residency
// cost. AI OCR, a disabled module and a failed configuration never warm Python.
func (s *aiService) localOCRStandbyEnabled() (bool, error) {
	if s.captureEnabled == nil || !s.captureEnabled() {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		var err error
		s.store, err = ai.Open(s.storePath)
		if err != nil {
			return false, err
		}
	}
	return s.store.Configuration().OCREngine != "ai", nil
}

func (s *aiService) logOCRPreparation(err error) {
	if err != nil && !errors.Is(err, context.Canceled) && s.ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "OCR 待命准备：", err)
	}
}

func (s *aiService) refreshLocalOCRStandby() {
	if s.localOCR == nil {
		return
	}
	enabled, err := s.localOCRStandbyEnabled()
	s.logOCRPreparation(err)
	// Intent is synchronous, inference is not. Configuration saves and the
	// main window remain responsive; disabling waits only in this background job.
	done := s.localOCR.SetStandby(enabled && err == nil)
	go func() { s.logOCRPreparation(<-done) }()
}

// Windows resume/unlock only asks to refault the existing model sessions. It
// neither changes the user's engine choice nor revives a disabled module.
func (s *aiService) prepareLocalOCRAfterResume() (bool, error) {
	enabled, err := s.localOCRStandbyEnabled()
	if err != nil || !enabled || s.localOCR == nil {
		return false, err
	}
	go func() { s.logOCRPreparation(s.localOCR.Prepare(s.ctx)) }()
	return true, nil
}
