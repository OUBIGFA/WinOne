package aiservices

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

//go:embed gateway_examples.json
var gatewayExamplesJSON []byte

type gatewayExample struct {
	ID, Name, Note, BaseSuffix string
	Decide                     bool
	Examples                   map[string]map[string]string
}

var gatewayExamples = func() []gatewayExample {
	var out []gatewayExample
	if err := json.Unmarshal(gatewayExamplesJSON, &out); err != nil {
		panic(err)
	}
	return out
}()

// The templates are generated from the selected upstream's actual builders.
// A copied example carries a key only after the owner explicitly reveals it.
func (g *gatewayPanel) connectView(c *ui.Context, providers []widgets.AIProvider, busy bool) {
	ui.Collapsible(c, upstreamT("Connect a client"), &g.connectOpen, func() {
		g.credentialsView(c, busy)
		if g.flavor == "" {
			g.flavor = "openai"
		}
		if g.snippetLanguage == "" {
			g.snippetLanguage = "shell"
		}
		if g.snippetOS == "" {
			g.snippetOS = "windows"
		}
		flavors := []kit.SelectOption{}
		for _, f := range gatewayExamples {
			flavors = append(flavors, kit.SelectOption{Value: f.ID, Label: f.Name})
		}
		choiceRow(c, upstreamT("API"), "", &g.flavor, flavors, busy)
		f := gatewayExamples[0]
		for _, entry := range gatewayExamples {
			if entry.ID == g.flavor {
				f = entry
			}
		}
		kit.Help(c, upstreamT(f.Note))
		root := strings.TrimSuffix(g.saved.BaseURL, "/v1")
		address := root + f.BaseSuffix
		kit.Row(c, upstreamT("Base URL"), address, func() {
			if kit.IconButton(c, icons.Copy, nativeT("copy")).Clicked() {
				c.Services().WriteClipboard(address)
			}
		})
		var models []kit.SelectOption
		for _, p := range providers {
			if p.Off {
				continue
			}
			for _, m := range p.Models {
				models = append(models, kit.SelectOption{Value: p.ID + "/" + m, Label: p.Name + " / " + widgets.ModelLabel(&p, m)})
			}
		}
		if g.exampleModel == "" && len(models) > 0 {
			g.exampleModel = models[0].Value
		}
		nativeSelect(c, upstreamT("Model"), &g.exampleModel, models, busy)
		languages := options("shell", "PowerShell", "curl", "curl", "python", "Python", "node", "Node")
		if f.Decide {
			languages = languages[1:]
			if g.snippetLanguage == "shell" {
				g.snippetLanguage = "curl"
			}
		}
		choiceRow(c, upstreamT("Example"), "", &g.snippetLanguage, languages, busy)
		if g.snippetLanguage == "shell" || g.snippetLanguage == "curl" {
			choiceRow(c, upstreamT("Shell"), "", &g.snippetOS, options("windows", "Windows", "unix", "macOS · Linux"), busy)
		}
		key := g.key
		if key == "" {
			key = "YOUR_API_KEY"
		}
		model := g.exampleModel
		if model == "" {
			model = "provider/model"
		}
		// These slots are inside code string literals (JSON, Python or JavaScript).
		// Quoting the model keeps a vendor-provided name from becoming pasted code.
		quoted, _ := json.Marshal(model)
		model = string(quoted[1 : len(quoted)-1])
		if g.snippetLanguage == "curl" && g.snippetOS == "unix" {
			model = strings.ReplaceAll(model, "'", "'\\''")
		}
		quoted, _ = json.Marshal(key)
		key = string(quoted[1 : len(quoted)-1])
		snippet := strings.NewReplacer("WINONE_ROOT", root, "WINONE_MODEL", model, "WINONE_KEY", key).Replace(f.Examples[g.snippetOS][g.snippetLanguage])
		kit.Mono(c, snippet).FillWidth().MinWidth(0).Selectable()
		if g.key == "" {
			kit.Help(c, i18n.T("gateway.keyHint"))
		}
		if kit.Button(c, nativeT("copy"), icons.Copy, false).Disabled(busy).Clicked() {
			c.Services().WriteClipboard(snippet)
		}
	})
}
