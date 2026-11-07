package aiservices

import (
	"encoding/json"
	"slices"
)

func routeSession(r traceRoute) string {
	if r.ParentSession != "" {
		return r.ParentSession
	}
	return r.Session
}

func routeTitleKey(r traceRoute) string {
	session := routeSession(r)
	if r.Agent != "codex" && r.NativeSession != "" {
		session = r.NativeSession
	}
	if session == "" {
		return ""
	}
	return r.Agent + ":" + session
}

// Magpie resolves Codex parents/names and other agents' native titles through
// distinct request shapes. Batch both at its 2,000-row limit; a late empty
// response must not erase an already-known title.
func (t *nativeTrace) loadTitles(p *page, rows []traceRoute, codex bool) {
	if len(rows) == 0 {
		return
	}
	batch := rows[:min(2000, len(rows))]
	var ids, keys []string
	var routeIDs []int64
	for _, row := range batch {
		if codex && row.Agent == "codex" {
			routeIDs = append(routeIDs, row.ID)
			if id := routeSession(row); id != "" && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		} else if !codex && row.Agent != "codex" {
			if key := routeTitleKey(row); key != "" && !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
		}
	}
	next := func() {
		if !codex {
			t.loadTitles(p, rows, true)
		} else {
			t.loadTitles(p, rows[len(batch):], false)
		}
	}
	if len(ids) == 0 && len(keys) == 0 {
		next()
		return
	}
	params := map[string]any{"sessions": keys}
	if codex {
		params = map[string]any{"ids": ids, "routeIds": routeIDs, "groups": true, "day": t.day}
	}
	t.titleAPI.request(p, "POST", "/api/gateway/session-titles", params, func(raw json.RawMessage) {
		var value struct {
			Titles        map[string]string `json:"titles"`
			Names         map[string]string `json:"names"`
			Parents       map[int64]string  `json:"parents"`
			Matched       map[int64]bool    `json:"matched"`
			ResetInferred bool              `json:"resetInferred"`
		}
		if !nativeDecode(&t.titleAPI, raw, &value) {
			return
		}
		if t.titles == nil {
			t.titles = map[string]string{}
		}
		for key, title := range value.Titles {
			if title != "" {
				t.titles[key] = title
			}
		}
		for id, title := range value.Names {
			if title != "" {
				t.titles["codex:"+id] = title
			}
		}
		for i := range t.routes {
			row := &t.routes[i]
			if row.Agent != "codex" || !slices.Contains(routeIDs, row.ID) {
				continue
			}
			if value.ResetInferred && row.ParentMatched {
				row.ParentSession, row.ParentMatched = "", false
			}
			if parent, ok := value.Parents[row.ID]; ok {
				row.ParentSession, row.ParentMatched = parent, value.Matched[row.ID]
			}
		}
		next()
	})
}
