//go:build windows

package ai

// MagpieRuntime supplies only the retained settings. Provider credentials are
// resolved separately for the selected connection, never from retired services.
func (s *Store) MagpieRuntime() (*MagpieSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configurationLocked().MagpieSettings.clone(), nil
}
func (s *Store) runtimeMagpieSettingsLocked(c Configuration) (*MagpieSettings, error) {
	return c.MagpieSettings.clone(), nil
}
