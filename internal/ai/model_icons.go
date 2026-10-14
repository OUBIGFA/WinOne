package ai

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
)

// One local rule inventory is also imported by the settings UI. It follows
// LobeHub's ModelIcon family matching approach without using the connection's
// branding to guess a model. Unknown deployment aliases stay generic.
//
//go:embed model-icons.json
var modelIconData []byte

var modelIconRules = func() []struct {
	pattern *regexp.Regexp
	icon    string
} {
	var source []struct {
		Pattern string `json:"pattern"`
		Icon    string `json:"icon"`
	}
	if err := json.Unmarshal(modelIconData, &source); err != nil {
		panic(err)
	}
	rules := make([]struct {
		pattern *regexp.Regexp
		icon    string
	}, len(source))
	for i, rule := range source {
		rules[i].pattern = regexp.MustCompile("(?i)" + rule.Pattern)
		rules[i].icon = rule.Icon
	}
	return rules
}()

// ModelIcon identifies the model family, independently of the serving provider.
func ModelIcon(model string) string {
	model = strings.TrimSpace(model)
	for _, rule := range modelIconRules {
		if rule.pattern.MatchString(model) {
			return rule.icon
		}
	}
	return "model"
}
