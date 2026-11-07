package automations

import (
	"encoding/json"
	"slices"
	"strings"
)

// Project previews are pushed facts, not reasons to ask pendingImports again:
// that read itself emits import-required and would form an endless refresh
// loop. Only schedule/runtime events require another list snapshot.
func (p *Page) receiveEvents(events []json.RawMessage) bool {
	changed := false
	for _, raw := range events {
		var event struct {
			Type                       string         `json:"type"`
			PendingImport              *PendingImport `json:"pendingImport"`
			WorkingDir                 string         `json:"workingDir"`
			Inserted, Updated, Deleted int
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			p.err = err.Error()
			continue
		}
		switch event.Type {
		case "import-required":
			if event.PendingImport == nil {
				continue
			}
			p.importsRevision++
			p.imports = slices.DeleteFunc(p.imports, func(item PendingImport) bool {
				return strings.EqualFold(item.WorkingDir, event.PendingImport.WorkingDir)
			})
			p.imports = append(p.imports, *event.PendingImport)
		case "reconciled":
			p.importsRevision++
			p.imports = slices.DeleteFunc(p.imports, func(item PendingImport) bool { return strings.EqualFold(item.WorkingDir, event.WorkingDir) })
			changed = changed || event.Inserted > 0 || event.Updated > 0 || event.Deleted > 0
		case "changed", "fired", "completed", "failed", "skipped", "session-bound", "runtime-state", "read", "all-read", "ready", "deferred":
			changed = true
		}
	}
	return changed
}
