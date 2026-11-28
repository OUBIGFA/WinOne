package ai

import (
	"maps"
	"slices"
)

// Copy owned data directly: serializing on every read allocates a second
// document and can silently erase invalid values before validation sees them.
// Keep this contract with provider_preferences.go in the embedded AI worker.
func (p ProviderPreferences) clone() ProviderPreferences {
	p.Contexts = maps.Clone(p.Contexts)
	p.MaxImages = maps.Clone(p.MaxImages)
	p.Addresses = slices.Clone(p.Addresses)
	p.AgentAccounts = maps.Clone(p.AgentAccounts)
	p.KeyBalanceAuth = maps.Clone(p.KeyBalanceAuth)
	for id, auth := range p.KeyBalanceAuth {
		auth.Token = cloneOption(auth.Token)
		p.KeyBalanceAuth[id] = auth
	}
	p.AccessKeyID = cloneOption(p.AccessKeyID)
	p.SecretAccessKey = cloneOption(p.SecretAccessKey)
	p.SessionToken = cloneOption(p.SessionToken)
	p.BalanceToken = cloneOption(p.BalanceToken)
	p.Endpoints = cloneOption(p.Endpoints)
	p.ZhipuTeam = cloneOption(p.ZhipuTeam)
	p.Vertex = cloneOption(p.Vertex)
	p.MaxAccounts = cloneOption(p.MaxAccounts)
	p.Fallback = slices.Clone(p.Fallback)
	p.AutoInclude = slices.Clone(p.AutoInclude)
	p.HiddenDrawers = slices.Clone(p.HiddenDrawers)
	p.HiddenVideomakers = slices.Clone(p.HiddenVideomakers)
	p.AccountConcurrency = maps.Clone(p.AccountConcurrency)
	p.AccountWeights = maps.Clone(p.AccountWeights)
	p.AccountModels = maps.Clone(p.AccountModels)
	for id, models := range p.AccountModels {
		p.AccountModels[id] = slices.Clone(models)
	}
	p.AccountProxies = maps.Clone(p.AccountProxies)
	p.AccountCaps = maps.Clone(p.AccountCaps)
	p.AccountWindowCaps = maps.Clone(p.AccountWindowCaps)
	for id, caps := range p.AccountWindowCaps {
		p.AccountWindowCaps[id] = maps.Clone(caps)
	}
	p.AccountWindowAmounts = maps.Clone(p.AccountWindowAmounts)
	for id, amounts := range p.AccountWindowAmounts {
		p.AccountWindowAmounts[id] = maps.Clone(amounts)
	}
	p.AccountNames = maps.Clone(p.AccountNames)
	p.KeySpendCaps = maps.Clone(p.KeySpendCaps)
	p.KeyOwnBalances = maps.Clone(p.KeyOwnBalances)
	for id, balance := range p.KeyOwnBalances {
		balance.CNY = cloneOption(balance.CNY)
		balance.Refill = cloneOption(balance.Refill)
		p.KeyOwnBalances[id] = balance
	}
	p.KeyPriceRates = maps.Clone(p.KeyPriceRates)
	p.KeyRateFactors = maps.Clone(p.KeyRateFactors)
	p.RateWindows = slices.Clone(p.RateWindows)
	for i := range p.RateWindows {
		p.RateWindows[i].Days = slices.Clone(p.RateWindows[i].Days)
	}
	p.ReplyHeaders = slices.Clone(p.ReplyHeaders)
	p.PassHeaders = slices.Clone(p.PassHeaders)
	p.ExtraBody = cloneBodyMap(p.ExtraBody)
	p.Resolve = maps.Clone(p.Resolve)
	for name, addresses := range p.Resolve {
		p.Resolve[name] = slices.Clone(addresses)
	}
	p.AccountModelsAll = maps.Clone(p.AccountModelsAll)
	return p
}

// These optional records contain only scalar fields. Nested records are copied
// explicitly above, making each mutable option's ownership visible here.
func cloneOption[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}

// ExtraBody comes from decoded JSON: only objects and arrays share storage.
// Preserve invalid scalar values as well so the save validator can reject them.
func cloneBodyMap(body map[string]any) map[string]any {
	body = maps.Clone(body)
	for key, value := range body {
		body[key] = cloneBodyValue(value)
	}
	return body
}

func cloneBodyValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneBodyMap(value)
	case []any:
		copy := slices.Clone(value)
		for i := range copy {
			copy[i] = cloneBodyValue(copy[i])
		}
		return copy
	default:
		return value
	}
}
