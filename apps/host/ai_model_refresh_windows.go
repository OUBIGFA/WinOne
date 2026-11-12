//go:build windows

package main

import (
	"context"
	"slices"
	"time"
	"winone/internal/ai"
)

// Refresh one saved provider. A concurrent user save wins through the store's
// revision check; a failed fetch retains the last usable catalog and picks.
func (s *aiService) refreshProviderModels(ctx context.Context, id string) error {
	store, err := s.magpieStore()
	if err != nil {
		return err
	}
	c := store.Configuration()
	at := slices.IndexFunc(c.Providers, func(p ai.Provider) bool { return p.ID == id })
	if at < 0 || c.Providers[at].ModelRefreshInterval() == 0 {
		return nil
	}
	if c.Providers[at].AuthMode == "account" {
		gate := s.accountGateFor(id)
		select {
		case gate <- struct{}{}:
			defer func() { <-gate }()
		default:
			return errMagpieAccountBusy
		}
	}
	p, keys, err := store.ProviderKeysForModels(id)
	if err != nil {
		return err
	}
	runtime, err := s.magpieTranslator()
	if err != nil {
		return err
	}
	var models []ai.ModelOption
	err = store.WithKeys(ctx, keys, func(ctx context.Context, key string) (bool, error) {
		var fetchErr error
		models, fetchErr = runtime.Models(ctx, p, key)
		return false, fetchErr
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	dst := &c.Providers[at]
	if err != nil {
		message := "模型自动刷新失败，请检查连接后重试"
		if dst.ModelRefreshError != message {
			dst.ModelRefreshError = message
			if _, saveErr := store.SaveConfiguration(c, nil); saveErr != nil {
				return saveErr
			}
		}
		return err
	}
	dst.RefreshModels(models)
	dst.ModelRefreshAt, dst.ModelRefreshError = time.Now(), ""
	if _, err = store.SaveConfiguration(c, nil); err != nil {
		return err
	}
	// Keep streams, trace and account affinity alive across background refreshes.
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()
	if s.backend.process != nil && s.backend.process.Alive() {
		if s.backend.pendingCatalog == nil {
			s.backend.pendingCatalog = map[string]ai.Provider{}
		}
		s.backend.pendingCatalog[dst.ID] = *dst
		if err := s.backend.process.SyncCatalog(ctx, *dst); err != nil {
			return err
		}
		delete(s.backend.pendingCatalog, dst.ID)
	}
	return nil
}
