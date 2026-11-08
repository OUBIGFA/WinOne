package aiservices

import (
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/ai"
)

// Include saved identities even when an account is offline. Merely opening
// this page must not drop policies for accounts absent from today's catalog.
func (e *editor) preferenceAccounts(pr *provider) []string {
	var ids []string
	if e.account != nil {
		for _, account := range e.account.accounts {
			ids = append(ids, strings.ToLower(account.User))
		}
	}
	if pr.AuthMode == "api-key" {
		for _, key := range pr.slots() {
			ids = append(ids, key.ID)
		}
	}
	s := &pr.ProviderPreferences
	for _, keys := range [][]string{sortedKeys(s.AccountNames), sortedKeys(s.AccountWeights), sortedKeys(s.AccountConcurrency), sortedKeys(s.AccountModels), sortedKeys(s.AccountModelsAll), sortedKeys(s.AccountProxies), sortedKeys(s.AccountCaps), sortedKeys(s.AccountWindowCaps), sortedKeys(s.KeySpendCaps), sortedKeys(s.KeyOwnBalances)} {
		ids = append(ids, keys...)
	}
	if e.preferenceAccount != "" {
		ids = append(ids, e.preferenceAccount)
	}
	return unique(ids)
}

func (e *editor) accountPreferences(c *ui.Context, pr *provider, locked bool) {
	s := &pr.ProviderPreferences
	ids := e.preferenceAccounts(pr)
	if !slices.Contains(ids, e.preferenceAccount) && len(ids) > 0 {
		e.preferenceAccount = ids[0]
	}
	ui.Column(c).FillWidth().Gap(12).Children(func() {
		kit.Help(c, i18n.T("ai.accountMapHint"))
		if len(ids) > 0 {
			nativeSelect(c, i18n.T("aiOptions.account"), &e.preferenceAccount, stringChoices(ids), locked)
		}
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			ui.TextInput(c, &e.newPreferenceAccount).Grow(1).MinWidth(160).Placeholder(i18n.T("aiOptions.accountID")).Label(i18n.T("aiOptions.accountID")).Disabled(locked)
			if kit.Button(c, i18n.T("aiOptions.addAccount"), icons.Plus, false).Disabled(locked || strings.TrimSpace(e.newPreferenceAccount) == "").Clicked() {
				e.preferenceAccount = strings.TrimSpace(e.newPreferenceAccount)
				if pr.AuthMode == "account" {
					e.preferenceAccount = strings.ToLower(e.preferenceAccount)
				}
				e.newPreferenceAccount = ""
			}
		})
	})
	id := e.preferenceAccount
	if id == "" {
		kit.Empty(c, icons.Users, i18n.T("aiOptions.noAccounts"), i18n.T("aiOptions.noAccountsHint"), nil)
		return
	}
	// Key the form by identity so a control's text cannot leak to another account.
	ui.Column(c.Key(id)).FillWidth().Gap(kit.GroupGap).Children(func() {
		kit.Group(c, nil, id, "", func() {
			kit.Stack(c, i18n.T("ai.accountNames"), "", func() {
				f := e.fields.get(id + ":name")
				if text, changed := f.sync(s.AccountNames[id]); changed {
					if s.AccountNames == nil {
						s.AccountNames = map[string]string{}
					}
					if text == "" {
						delete(s.AccountNames, id)
					} else {
						s.AccountNames[id] = text
					}
				}
				ui.TextInput(c, &f.text).FillWidth().Label(i18n.T("ai.accountNames")).Disabled(locked)
			})
			accountNumber(c, i18n.T("ai.accountWeights"), &s.AccountWeights, id, 1, 1000, locked)
			accountNumber(c, i18n.T("ai.accountConcurrency"), &s.AccountConcurrency, id, 0, 1024, locked)
			accountNumber(c, i18n.T("ai.accountCaps"), &s.AccountCaps, id, 1, 99, locked)
			kit.Stack(c, "", "", func() {
				proxy := s.AccountProxies[id]
				e.proxyPreference(c, id, &proxy, locked)
				if proxy != s.AccountProxies[id] {
					if s.AccountProxies == nil {
						s.AccountProxies = map[string]string{}
					}
					if proxy == "" {
						delete(s.AccountProxies, id)
					} else {
						s.AccountProxies[id] = proxy
					}
				}
			})
		})
		kit.Group(c, nil, i18n.T("ai.accountModels"), i18n.T("aiOptions.modelsHint"), func() {
			all := s.AccountModelsAll[id]
			if kit.Toggle(c, &all, nativeT("accountModelsAll"), nativeT("accountModelsAllHint")).Disabled(locked).Changed() {
				if s.AccountModelsAll == nil {
					s.AccountModelsAll = map[string]bool{}
				}
				if all {
					s.AccountModelsAll[id] = true
				} else {
					delete(s.AccountModelsAll, id)
				}
			}
			kit.Stack(c, "", "", func() {
				models := s.AccountModels[id]
				e.preferenceList(c, id+":models", i18n.T("ai.accountModels"), &models, locked)
				if !slices.Equal(models, s.AccountModels[id]) {
					if s.AccountModels == nil {
						s.AccountModels = map[string][]string{}
					}
					if len(models) == 0 {
						delete(s.AccountModels, id)
					} else {
						s.AccountModels[id] = models
					}
				}
			})
		})
		e.accountWindowCaps(c, pr, id, locked)
		if pr.AuthMode == "api-key" {
			e.keySpending(c, pr, id, locked)
		}
		if kit.GhostButton(c, i18n.T("aiOptions.removeAccount"), icons.RotateCcw, false).Disabled(locked).Clicked() {
			delete(s.AccountNames, id)
			delete(s.AccountWeights, id)
			delete(s.AccountConcurrency, id)
			delete(s.AccountCaps, id)
			delete(s.AccountProxies, id)
			delete(s.AccountModels, id)
			delete(s.AccountModelsAll, id)
			delete(s.AccountWindowCaps, id)
			delete(s.KeySpendCaps, id)
			delete(s.KeyOwnBalances, id)
			for key := range e.fields {
				if strings.Contains(key, id) {
					delete(e.fields, key)
				}
			}
		}
	})
}

func accountNumber(c *ui.Context, label string, values *map[string]int, id string, low, high float64, locked bool) {
	var current *float64
	if v, ok := (*values)[id]; ok {
		n := float64(v)
		current = &n
	}
	numberRow(c, label, current, low, high, true, i18n.T("aiOptions.default"), locked, func(v *float64) {
		if *values == nil {
			*values = map[string]int{}
		}
		if v == nil {
			delete(*values, id)
		} else {
			(*values)[id] = int(*v)
		}
	})
}

func (e *editor) accountWindowCaps(c *ui.Context, pr *provider, id string, locked bool) {
	s := &pr.ProviderPreferences
	kit.Group(c, nil, i18n.T("ai.accountWindowCaps"), i18n.T("ai.windowCapsHint"), func() {
		caps := s.AccountWindowCaps[id]
		if len(caps) == 0 {
			kit.Stack(c, "", i18n.T("aiOptions.noWindowCaps"), func() {})
		}
		for _, name := range sortedKeys(caps) {
			accountNumber(c, name, &caps, name, 1, 100, locked)
		}
		kit.Stack(c, "", "", func() {
			ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
				ui.TextInput(c, &e.newCapWindow).Grow(1).MinWidth(140).Label(i18n.T("aiOptions.windowName")).Placeholder(i18n.T("aiOptions.windowName")).Disabled(locked)
				if kit.Button(c, i18n.T("aiOptions.addCap"), icons.Plus, false).Disabled(locked || strings.TrimSpace(e.newCapWindow) == "").Clicked() {
					if caps == nil {
						caps = map[string]int{}
					}
					caps[strings.TrimSpace(e.newCapWindow)] = 100
					e.newCapWindow = ""
				}
			})
		})
		if len(caps) > 0 {
			if s.AccountWindowCaps == nil {
				s.AccountWindowCaps = map[string]map[string]int{}
			}
			s.AccountWindowCaps[id] = caps
		} else {
			delete(s.AccountWindowCaps, id)
		}
	})
}

func (e *editor) keySpending(c *ui.Context, pr *provider, id string, locked bool) {
	s := &pr.ProviderPreferences
	kit.Group(c, nil, nativeT("ownBalance"), nativeT("ownBalanceHint"), func() {
		balance, has := s.KeyOwnBalances[id]
		var amount *float64
		if has {
			amount = &balance.USD
		}
		numberRow(c, nativeT("ownBalanceUSD"), amount, 0, 1000000, false, i18n.T("aiOptions.default"), locked, func(v *float64) {
			if v == nil {
				delete(s.KeyOwnBalances, id)
				return
			}
			if s.KeyOwnBalances == nil {
				s.KeyOwnBalances = map[string]ai.OwnBalance{}
			}
			s.KeyOwnBalances[id] = ai.OwnBalance{USD: *v, At: time.Now().UTC()}
		})
		if has && kit.Button(c, nativeT("resetBalanceTime"), icons.RefreshCw, false).Disabled(locked).Clicked() {
			balance.At = time.Now().UTC()
			s.KeyOwnBalances[id] = balance
		}
	})
	kit.Group(c, nil, i18n.T("ai.keySpendCaps"), i18n.T("ai.keySpendCapsHint"), func() {
		cap := s.KeySpendCaps[id]
		var amount *float64
		if cap.USD > 0 {
			amount = &cap.USD
		}
		numberRow(c, i18n.T("ai.keySpendCaps"), amount, .001, 1000000, false, i18n.T("aiOptions.default"), locked, func(v *float64) { cap.USD = value(v) })
		if cap.Period == "" {
			cap.Period = "month"
		}
		choiceRow(c, i18n.T("aiOptions.period"), "", &cap.Period, options("day", i18n.T("aiOptions.day"), "month", i18n.T("aiOptions.month"), "total", i18n.T("aiOptions.total")), locked)
		if cap.USD > 0 {
			if s.KeySpendCaps == nil {
				s.KeySpendCaps = map[string]ai.SpendCap{}
			}
			s.KeySpendCaps[id] = cap
		} else {
			delete(s.KeySpendCaps, id)
		}
	})
}
