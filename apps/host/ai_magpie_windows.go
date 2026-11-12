//go:build windows

package main

import (
	"errors"
	"winone/internal/ai"
)

func (s *aiService) magpieStore() (*ai.Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		var err error
		s.store, err = ai.Open(s.storePath)
		if err != nil {
			return nil, errors.New("AI 设置无法读取，请恢复配置文件后重试：" + err.Error())
		}
	}
	return s.store, nil
}

func (s *aiService) magpieTranslator() (ai.Translator, error) {
	store, err := s.magpieStore()
	if err != nil {
		return ai.Translator{}, err
	}
	prefs, err := store.MagpieRuntime()
	if err != nil {
		return ai.Translator{}, err
	}
	return s.translator.WithMagpieSettings(prefs), nil
}
