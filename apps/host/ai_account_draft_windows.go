//go:build windows

package main

import (
	"errors"
	"winone/internal/ai"
)

func (s *aiService) accountProvider(id, agent string, draft *ai.AccountDraft) (ai.Provider, error) {
	s.mu.Lock()
	var err error
	if s.store == nil {
		s.store, err = ai.Open(s.storePath)
	}
	s.mu.Unlock()
	if err != nil {
		return ai.Provider{}, err
	}
	provider, exists := s.store.Configuration().Provider(id)
	if draft != nil {
		if draft.Agent != agent || id == "" || id == "legacy" && !exists {
			return ai.Provider{}, errors.New("账号连接标识无效")
		}
		if exists && provider.AuthMode != "account" {
			return ai.Provider{}, errors.New("此供应商不是账号连接")
		}
		return draft.Provider(id)
	}
	if !exists || provider.AuthMode != "account" || provider.Agent != agent {
		return ai.Provider{}, errors.New("所选账号服务不可用，请重新选择")
	}
	return provider, nil
}
