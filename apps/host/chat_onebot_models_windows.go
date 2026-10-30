//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"winone/internal/ai"
	"winone/internal/chat"
)

type oneBotModelRoute struct {
	Harness    string          `json:"harness"`
	ProviderID string          `json:"providerId"`
	Model      string          `json:"model"`
	Effort     string          `json:"effort"`
	FastMode   json.RawMessage `json:"fastMode"`
}
type oneBotModelPlan struct {
	Key           string
	Chain         []oneBotModelRoute
	Model         ai.Selection
	ThinkingLevel string
	FastMode      bool
}

// Match Cindy's durable follow-default marker and preserve the complete chain.
// A runtime-only fallback/manual choice survives until the configured chain
// changes. Never manufacture a route from unrelated enabled models.
func resolveOneBotModel(value oneBotContext, configuration ai.Configuration, current chat.Conversation) (oneBotModelPlan, error) {
	capabilities := value.Capabilities
	chain := append([]oneBotModelRoute{}, capabilities.ModelChain...)
	follows := string(capabilities.ModelChainOverride) == "null" || string(capabilities.ModelOverride) == "null"
	if len(capabilities.ModelChainOverride) > 0 && string(capabilities.ModelChainOverride) != "null" {
		if err := json.Unmarshal(capabilities.ModelChainOverride, &chain); err != nil {
			return oneBotModelPlan{}, err
		}
		follows = false
	}
	if follows || len(chain) == 0 && capabilities.Model == "" {
		chain = append([]oneBotModelRoute{}, value.DefaultModelChain...)
		if len(chain) == 0 {
			chain = oneBotDefaultModelChain(configuration, chat.Preferences{})
		}
	} else if len(chain) == 0 {
		chain = []oneBotModelRoute{{Harness: "pi", ProviderID: capabilities.ProviderID, Model: capabilities.Model, Effort: capabilities.Effort, FastMode: capabilities.FastMode}}
	}
	if len(chain) == 0 || len(chain) > 5 {
		return oneBotModelPlan{}, errors.New("OneBot 模型链需要 1 至 5 条路线")
	}
	for i := range chain {
		if chain[i].Harness == "" {
			chain[i].Harness = "pi"
		}
		if chain[i].Harness != "pi" {
			return oneBotModelPlan{}, errors.New("OneBot 仅使用 Pi")
		}
		if len(chain[i].FastMode) == 0 || string(chain[i].FastMode) == "null" {
			chain[i].FastMode = json.RawMessage("false")
		}
	}
	encoded, err := json.Marshal(chain)
	if err != nil {
		return oneBotModelPlan{}, err
	}
	digest := sha256.Sum256(encoded)
	plan := oneBotModelPlan{Key: hex.EncodeToString(digest[:]), Chain: chain}
	start := 0
	if current.OneBotModelKey == plan.Key {
		if configuration.ValidateSelection(current.Model) == nil {
			plan.Model, plan.ThinkingLevel, plan.FastMode = current.Model, current.ThinkingLevel, current.FastMode
			return plan, nil
		}
		start = oneBotModelRouteIndex(chain, current.Model) + 1
	}
	for _, route := range chain[start:] {
		selection, fast, err := oneBotRouteSelection(route, configuration)
		if err != nil {
			continue
		}
		plan.Model, plan.ThinkingLevel, plan.FastMode = selection, route.Effort, fast
		return plan, nil
	}
	return oneBotModelPlan{}, errors.New("OneBot 模型链没有可用路线，请在 AI 服务中启用所选模型")
}

func oneBotDefaultModelChain(configuration ai.Configuration, prefs chat.Preferences) []oneBotModelRoute {
	selected := configuration.Selected("chat")
	return []oneBotModelRoute{{Harness: "pi", ProviderID: selected.Provider, Model: selected.Model, Effort: prefs.DefaultThinkingLevel, FastMode: json.RawMessage("false")}}
}

// Resolve implicit providers at the same Host boundary as the initial route.
// Unavailable entries remain in order; the Worker skips entries absent from
// its enabled model catalog instead of inventing a replacement.
func oneBotResolvedModelChain(plan oneBotModelPlan, configuration ai.Configuration) []oneBotModelRoute {
	chain := append([]oneBotModelRoute{}, plan.Chain...)
	for i, route := range chain {
		if selection, _, err := oneBotRouteSelection(route, configuration); err == nil {
			chain[i].ProviderID = selection.Provider
		}
	}
	return chain
}

func oneBotPermission(value string) string {
	if value == "trusted" || value == "bypassPermissions" {
		return "bypassPermissions"
	}
	if value == "auto" {
		return "auto"
	}
	return "ask"
}

func (b *chatOneBot) prepareModelConversation(conversation chat.Conversation, value oneBotContext) (chat.Conversation, error) {
	configuration, err := b.control.aiConfiguration()
	if err != nil {
		return chat.Conversation{}, err
	}
	value.DefaultModelChain = b.defaultModelChain(configuration)
	plan, err := resolveOneBotModel(value, configuration, conversation)
	if err != nil {
		return chat.Conversation{}, err
	}
	permission := oneBotPermission(value.Capabilities.Permissions)
	if conversation.Model == plan.Model && conversation.ThinkingLevel == plan.ThinkingLevel && conversation.FastMode == plan.FastMode && conversation.OneBotModelKey == plan.Key && conversation.Permission == permission {
		return conversation, nil
	}
	return b.sessions.store.UpdateConversation(conversation.ID, func(c *chat.Conversation) error {
		c.Model, c.ThinkingLevel, c.FastMode, c.OneBotModelKey, c.Permission = plan.Model, plan.ThinkingLevel, plan.FastMode, plan.Key, permission
		return nil
	})
}

func oneBotRouteSelection(route oneBotModelRoute, configuration ai.Configuration) (ai.Selection, bool, error) {
	var fast bool
	if len(route.FastMode) > 0 && string(route.FastMode) != "null" {
		if err := json.Unmarshal(route.FastMode, &fast); err != nil {
			return ai.Selection{}, false, errors.New("Pi 路线不支持 ultrafast")
		}
	}
	selection := ai.Selection{Provider: route.ProviderID, Model: route.Model}
	if selection.Provider == "" {
		fallback := configuration.Selected("chat")
		if fallback.Model == route.Model && configuration.ValidateSelection(fallback) == nil {
			selection = fallback
		} else {
			for _, provider := range configuration.Providers {
				if slices.Contains(provider.Models, route.Model) {
					if selection.Provider != "" {
						return ai.Selection{}, false, errors.New("该模型有多个来源，请指定供应商")
					}
					selection.Provider = provider.ID
				}
			}
		}
	}
	return selection, fast, configuration.ValidateSelection(selection)
}

// Same ordered identity rule as botModelRouteIndex: exact source first, then
// a unique implicit source, then a unique legacy model identity.
func oneBotModelRouteIndex(chain []oneBotModelRoute, current ai.Selection) int {
	implicit, legacy := -1, -1
	implicitCount, legacyCount := 0, 0
	for i, route := range chain {
		if route.Model != current.Model {
			continue
		}
		if route.ProviderID == current.Provider {
			return i
		}
		legacy = i
		legacyCount++
		if route.ProviderID == "" {
			implicit = i
			implicitCount++
		}
	}
	if implicitCount == 1 {
		return implicit
	}
	if legacyCount == 1 {
		return legacy
	}
	return -1
}
