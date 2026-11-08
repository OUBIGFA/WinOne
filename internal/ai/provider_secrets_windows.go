//go:build windows

package ai

import "errors"

// Keep supplemental vendor credentials out of ordinary configuration JSON,
// exports and logs. Nil preserves an existing secret; an empty edit clears it.
func (s *Store) prepareProviderSecrets(c *Configuration) (map[string]map[string]string, error) {
	sealed := map[string]map[string]string{}
	for i := range c.Providers {
		p := &c.Providers[i]
		fields := []struct {
			name string
			edit **string
			has  *bool
		}{
			{"accessKeyID", &p.AccessKeyID, &p.HasAccessKeyID}, {"secretAccessKey", &p.SecretAccessKey, &p.HasSecretAccessKey}, {"balanceToken", &p.BalanceToken, &p.HasBalanceToken},
		}
		values := map[string]string{}
		for _, f := range fields {
			value := s.providerSecrets[p.ID][f.name]
			if *f.edit != nil {
				if p.AuthMode != "api-key" {
					return nil, errors.New("供应商专用密钥仅用于自定义供应商")
				}
				var err error
				value, err = sealCredential(**f.edit)
				if err != nil {
					return nil, err
				}
			}
			*f.edit = nil
			if p.AuthMode != "api-key" {
				value = ""
			}
			*f.has = value != ""
			if value != "" {
				values[f.name] = value
			}
		}
		if len(values) > 0 {
			sealed[p.ID] = values
		}
	}
	return sealed, nil
}

// Only callers performing a vendor operation use this function. Preserve
// supplied draft edits so testing an unsaved credential never saves it.
func (s *Store) hydrateProviderSecrets(p *Provider) error {
	for _, f := range []struct {
		name  string
		value **string
	}{
		{"accessKeyID", &p.AccessKeyID}, {"secretAccessKey", &p.SecretAccessKey}, {"balanceToken", &p.BalanceToken},
	} {
		if *f.value != nil {
			continue
		}
		sealed := s.providerSecrets[p.ID][f.name]
		if sealed == "" {
			continue
		}
		value, err := unsealKey(sealed)
		if err != nil {
			return err
		}
		*f.value = &value
	}
	return nil
}
