package profiles

import (
	"encoding/json"
	"sort"
	"strings"
	"winone/internal/catalog"
	"winone/internal/hotkeys"
)

type ModulePlan struct {
	ID            string          `json:"id"`
	State         string          `json:"state"`
	MissingPaths  []string        `json:"missingPaths"`
	RequiredPaths []string        `json:"requiredPaths"`
	ArgumentApps  []string        `json:"argumentApps"`
	Error         string          `json:"error,omitempty"`
	Patch         json.RawMessage `json:"-"`
	Settings      json.RawMessage `json:"settings,omitempty"`
}
type ShortcutIssue struct {
	Action string `json:"action"`
	Other  string `json:"other,omitempty"`
	Kind   string `json:"kind"`
	Error  string `json:"error"`
}
type ArchivePlan struct {
	Modules        []ModulePlan    `json:"modules"`
	ShortcutIssues []ShortcutIssue `json:"shortcutIssues"`
}

// PrepareArchive reports each module independently. Unknown payloads remain in
// the original archive; it neither activates modules nor invents their schemas.
// Ready means data validation passed, not that device validation or apply ran.
func PrepareArchive(archive Archive, bindings map[string]string) (ArchivePlan, error) {
	if err := validate(archive); err != nil {
		return ArchivePlan{}, err
	}
	plan := ArchivePlan{Modules: []ModulePlan{}, ShortcutIssues: []ShortcutIssue{}}
	known := map[string]bool{}
	actions := map[string]bool{}
	for _, manifest := range catalog.Builtins() {
		known[manifest.ID] = true
		for _, action := range manifest.ShortcutActions {
			actions[manifest.ID+"."+action] = true
		}
	}
	ids := make([]string, 0, len(archive.Modules))
	for id := range archive.Modules {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		item := ModulePlan{ID: id, State: "missing_module", MissingPaths: []string{}, ArgumentApps: []string{}, RequiredPaths: []string{}}
		if known[id] {
			prepared, err := PrepareBuiltinImport(id, archive.Modules[id], archive.Paths, bindings)
			if err != nil {
				item.State = "invalid"
				item.Error = err.Error()
			} else {
				item.Settings = append(json.RawMessage(nil), archive.Modules[id].Settings...)
				item.State = "ready"
				item.MissingPaths = prepared.MissingPaths
				item.RequiredPaths = prepared.RequiredPaths
				item.ArgumentApps = prepared.ArgumentApps
				item.Patch = prepared.Patch
				if len(item.MissingPaths) > 0 {
					item.State = "needs_mapping"
				}
			}
		}
		plan.Modules = append(plan.Modules, item)
	}
	// Global bindings participate in the same normalized conflict analysis,
	// but remain separate from module selection in the archive and import UI.
	values := make(map[string]string, len(archive.Preferences.Shortcuts)+1)
	for id, key := range archive.Preferences.Shortcuts {
		values[id] = key
	}
	if archive.Preferences.OpenShortcut != nil {
		values["application.open"] = *archive.Preferences.OpenShortcut
		actions["application.open"] = true
	}
	shortcuts := make([]string, 0, len(values))
	for id := range values {
		shortcuts = append(shortcuts, id)
	}
	sort.Strings(shortcuts)
	used := map[hotkeys.Shortcut]string{}
	for _, id := range shortcuts {
		moduleKnown := false
		for module := range known {
			if strings.HasPrefix(id, module+".") {
				moduleKnown = true
				break
			}
		}
		if !actions[id] {
			kind, message := "missing_module", "快捷键所属模块尚未安装"
			if moduleKnown {
				kind = "unsupported"
				message = "模块不支持此快捷键操作"
			}
			plan.ShortcutIssues = append(plan.ShortcutIssues, ShortcutIssue{Action: id, Kind: kind, Error: message})
			continue
		}
		key, err := hotkeys.Parse(values[id])
		if err != nil {
			plan.ShortcutIssues = append(plan.ShortcutIssues, ShortcutIssue{Action: id, Kind: "invalid", Error: err.Error()})
			continue
		}
		if key.Key == 0 {
			continue
		}
		if other := used[key]; other != "" {
			plan.ShortcutIssues = append(plan.ShortcutIssues, ShortcutIssue{Action: id, Other: other, Kind: "conflict", Error: "快捷键与另一操作重复"})
		} else {
			used[key] = id
		}
	}
	return plan, nil
}
