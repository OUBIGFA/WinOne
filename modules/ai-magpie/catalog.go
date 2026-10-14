package main

import (
	_ "embed"
	"encoding/json"
)

// The build checks this registry and every plugin file against the pinned
// community revision. Listing accounts never loads code or contacts a vendor.
//
//go:embed plugin-catalog.json
var pluginCatalogJSON []byte

type accountPlugin struct {
	Package   string           `json:"package"`
	Name      string           `json:"name"`
	Version   string           `json:"version"`
	Directory string           `json:"directory"`
	Entry     string           `json:"entry"`
	Providers []string         `json:"providers"`
	Files     []pluginResource `json:"files"`
}

type pluginResource struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type pluginResourceCatalog struct {
	Bun struct {
		Version string `json:"version"`
		SHA256  string `json:"executableSHA256"`
	} `json:"bun"`
	Plugins []accountPlugin `json:"plugins"`
}

var bundledPluginResources = func() pluginResourceCatalog {
	var resources pluginResourceCatalog
	if err := json.Unmarshal(pluginCatalogJSON, &resources); err != nil {
		panic(err)
	}
	return resources
}()

func accountPlugins() []accountPlugin {
	return bundledPluginResources.Plugins
}

func accountCatalog() []agentOption {
	options := append([]agentOption(nil), agentOptions...)
	positions := map[string]int{}
	for index, option := range options {
		positions[option.ID] = index
	}
	for _, plugin := range accountPlugins() {
		for _, id := range plugin.Providers {
			if index, found := positions[id]; found {
				// One provider stays one choice even when both implementations
				// exist. The explicit backend applies to newly selected accounts;
				// old saved accounts retain their original built-in connection.
				options[index].Backend, options[index].Plugin = "plugin", plugin.Package
				continue
			}
			name := plugin.Name
			switch id {
			case "minimax-code-global":
				name = "MiniMax Code International"
			case "trae-cn":
				name = "Trae CN"
			case "trae-global":
				name = "Trae Global"
			}
			positions[id] = len(options)
			options = append(options, agentOption{ID: id, Name: name, Backend: "plugin", Plugin: plugin.Package})
		}
	}
	return options
}

func pluginForAgent(agent string) (accountPlugin, bool) {
	for _, plugin := range accountPlugins() {
		for _, id := range plugin.Providers {
			if id == agent {
				return plugin, true
			}
		}
	}
	return accountPlugin{}, false
}
