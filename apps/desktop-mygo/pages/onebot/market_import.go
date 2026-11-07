package onebot

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type MarketCopy struct {
	Name, Description, Welcome, Bubble string
	Capabilities                       []struct{ Icon, Label string }
	Skills                             []struct{ Name, Description string }
	Knowledge                          []struct{ Title, Summary string }
	Remembers                          []string
	Routines                           []struct{ Name, Summary, Cron string }
	Tools                              []struct{ ID, Purpose string }
	ItemNames                          map[string]string
}
type MarketEntry struct {
	PackageID, PackageVersion, PackageHash, Avatar string
	MarketCopy
	Locales map[string]MarketCopy
}
type ImportCheck struct {
	EntryID, Status, Message string
	Progress                 *struct{ Saved, Total int }
}
type ImportResult struct {
	RequestID, BotID, CanonicalSessionID, Status string
	Checks                                       []ImportCheck
	Saved                                        bool
	SavedEntryIDs                                []string
}
type marketState struct {
	entries       []MarketEntry
	selected      string
	loading, busy bool
	error         string
	receipt       *ImportResult
	pollCancel    func()
}
type ImportSource struct{ ID, Kind, Name string }
type ImportEntry struct {
	ID, Category, Name, Description  string
	Selected                         bool
	Enabled                          *bool
	DependsOn, ExclusiveWith, Issues []string
}
type ImportPreview struct {
	ID, Name                         string
	Source                           ImportSource
	AvatarImageBase64                string
	Entries                          []ImportEntry
	SelectionRanges, SelectionChunks bool
}
type importState struct {
	sources              []ImportSource
	preview              *ImportPreview
	selected             []string
	name, requestID      string
	takeover, open, busy bool
	error                string
	receipt              *ImportResult
	pollCancel           func()
}

func (p *page) loadMarket() {
	if p.market.loading {
		return
	}
	p.market.loading = true
	oneBotCall(p, "market.list", nil, func(entries []MarketEntry, err error) {
		p.market.loading = false
		if err != nil {
			p.market.error = err.Error()
			return
		}
		p.market.entries = entries
		p.market.error = ""
		for _, entry := range entries {
			p.avatars.load(p, []Profile{{ID: entry.PackageID, Name: entry.Name, Avatar: entry.Avatar}})
		}
	})
}
func localizedMarket(entry MarketEntry) MarketEntry {
	if botLocale() == "en" {
		if copy, ok := entry.Locales["en"]; ok {
			entry.MarketCopy = copy
		}
	}
	return entry
}
func (p *page) marketView(c *ui.Context) {
	m := &p.market
	if len(m.entries) == 0 && !m.loading && m.error == "" {
		p.loadMarket()
	}
	if m.loading {
		kit.Loader(c)
	}
	kit.InlineError(c, m.error)
	ui.Scroll(c).Fill().MinHeight(0).Gap(10).Children(func() {
		for _, raw := range m.entries {
			entry := localizedMarket(raw)
			if m.selected != "" && m.selected != entry.PackageID {
				continue
			}
			ui.Column(c.Key(entry.PackageID)).FillWidth().Gap(8).Padding(12, 0).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
				ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
					p.avatars.View(c, Profile{ID: entry.PackageID, Name: entry.Name, Avatar: entry.Avatar}, 48)
					ui.Column(c).Grow(1).MinWidth(0).Children(func() { kit.Strong(c, entry.Name); kit.Help(c, entry.Description) })
				})
				if m.selected == "" {
					if kit.Button(c, nt("details"), icons.ChevronRight, false).Clicked() {
						m.selected = entry.PackageID
					}
					return
				}
				if kit.Button(c, nt("back"), icons.ArrowLeft, false).Clicked() {
					m.selected = ""
				}
				kit.Help(c, entry.Bubble)
				kit.Body(c, entry.Welcome)
				for _, capability := range entry.Capabilities {
					kit.Help(c, capability.Label)
				}
				for _, skill := range entry.Skills {
					kit.Strong(c, skill.Name)
					kit.Help(c, skill.Description)
				}
				for _, knowledge := range entry.Knowledge {
					kit.Strong(c, knowledge.Title)
					kit.Help(c, knowledge.Summary)
				}
				for _, remembers := range entry.Remembers {
					kit.Help(c, remembers)
				}
				for _, routine := range entry.Routines {
					kit.Strong(c, routine.Name)
					kit.Help(c, routine.Summary)
					if routine.Cron != "" {
						kit.Mono(c, routine.Cron)
					}
				}
				for _, tool := range entry.Tools {
					kit.Strong(c, tool.ID)
					kit.Help(c, tool.Purpose)
				}
				if kit.PrimaryButton(c, nt("invite"), icons.UserPlus, m.busy).Disabled(m.busy).Clicked() {
					p.invitePackage(entry)
				}
			})
		}
		p.importReceiptView(c, m.receipt, nil)
	})
}
func (p *page) invitePackage(entry MarketEntry) {
	m := &p.market
	if m.busy {
		return
	}
	m.busy = true
	m.error = ""
	oneBotCall(p, "market.invite", map[string]any{"input": map[string]string{"packageId": entry.PackageID, "packageVersion": entry.PackageVersion, "packageHash": entry.PackageHash, "locale": botLocale()}}, func(receipt ImportResult, err error) {
		m.busy = false
		if err != nil {
			m.error = err.Error()
			return
		}
		m.receipt = &receipt
		p.refresh()
		p.pollImportReceipt("market", receipt.RequestID)
	})
}
func (p *page) beginImport() {
	p.env.RequestChange(func() { p.importing.open = true })
	oneBotCall(p, "import.sources", nil, func(sources []ImportSource, err error) {
		if err != nil {
			p.importing.error = err.Error()
			return
		}
		p.importing.sources = sources
	})
}
func (p *page) previewImport(source ImportSource) {
	s := &p.importing
	if s.busy {
		return
	}
	s.busy = true
	oneBotCall(p, "import.preview", map[string]string{"sourceId": source.ID}, func(preview ImportPreview, err error) {
		s.busy = false
		if err != nil {
			s.error = err.Error()
			return
		}
		s.preview = &preview
		s.name = preview.Name
		s.requestID = rand.Text()
		s.selected = nil
		s.receipt = nil
		s.error = ""
		for _, entry := range preview.Entries {
			if entry.Selected {
				s.selected = append(s.selected, entry.ID)
			}
		}
	})
}
func toggleImportEntries(entries []ImportEntry, current, ids []string, checked bool) []string {
	if !checked {
		return slices.DeleteFunc(append([]string(nil), current...), func(id string) bool { return slices.Contains(ids, id) })
	}
	selected := append([]string(nil), current...)
	if len(ids) == 1 {
		for _, entry := range entries {
			if entry.ID == ids[0] {
				selected = slices.DeleteFunc(selected, func(id string) bool { return slices.Contains(entry.ExclusiveWith, id) })
			}
		}
	}
	for _, id := range ids {
		if slices.Contains(selected, id) {
			continue
		}
		var entry *ImportEntry
		for i := range entries {
			if entries[i].ID == id {
				entry = &entries[i]
				break
			}
		}
		if entry == nil {
			continue
		}
		conflicting := false
		if len(ids) > 1 {
			for _, other := range entry.ExclusiveWith {
				if slices.Contains(selected, other) || slices.Contains(ids, other) {
					conflicting = true
					break
				}
			}
		}
		if !conflicting {
			selected = append(selected, id)
		}
	}
	return selected
}
func compactImportSelection(preview ImportPreview, selected []string) map[string]any {
	out := map[string]any{"entryIds": selected}
	if !preview.SelectionRanges || len(selected) < 200 {
		return out
	}
	ranges := [][2]int{}
	for index, entry := range preview.Entries {
		if !slices.Contains(selected, entry.ID) {
			continue
		}
		last := len(ranges) - 1
		if last >= 0 && ranges[last][1]+1 == index {
			ranges[last][1] = index
		} else {
			ranges = append(ranges, [2]int{index, index})
		}
	}
	raw, _ := json.Marshal(ranges)
	ids, _ := json.Marshal(selected)
	if len(raw) < len(ids) {
		out["entryIds"] = []string{}
		out["entryRanges"] = ranges
	}
	return out
}

// Chunk offsets are JavaScript UTF-16 units, as in the upstream protocol.
// Keep surrogate pairs together while preserving the exact reassembled JSON.
func importSubmissions(selection map[string]any, chunked bool) []map[string]any {
	raw, _ := json.Marshal(selection)
	units := utf16.Encode([]rune(string(raw)))
	if !chunked || len(units) <= 8192 {
		return []map[string]any{selection}
	}
	out := []map[string]any{}
	for offset := 0; offset < len(units); {
		end := min(offset+8192, len(units))
		if end < len(units) && units[end-1] >= 0xd800 && units[end-1] <= 0xdbff {
			end--
		}
		text := string(utf16.Decode(units[offset:end]))
		out = append(out, map[string]any{"selectionChunk": map[string]any{"id": selection["requestId"], "offset": offset, "total": len(units), "text": text}})
		offset = end
	}
	return out
}
func (p *page) submitImport() {
	s := &p.importing
	if s.busy || s.preview == nil {
		return
	}
	s.busy = true
	s.error = ""
	preview := copyValue(*s.preview)
	selection := compactImportSelection(preview, s.selected)
	selection["previewId"], selection["requestId"], selection["name"], selection["takeover"], selection["deferSetup"] = preview.ID, s.requestID, s.name, s.takeover, true
	if preview.AvatarImageBase64 != "" {
		selection["avatarImageBase64"] = preview.AvatarImageBase64
	}
	core.Go(p.env, func() (ImportResult, error) {
		var receipt ImportResult
		for _, part := range importSubmissions(selection, preview.SelectionChunks) {
			if err := p.call("import.start", map[string]any{"selection": part}, &receipt); err != nil {
				return receipt, err
			}
		}
		return receipt, nil
	}, func(receipt ImportResult, err error) {
		if p.closed {
			return
		}
		s.busy = false
		if err != nil {
			s.error = err.Error()
			return
		}
		s.receipt = &receipt
		p.refresh()
		p.pollImportReceipt("import", receipt.RequestID)
	})
}
func (p *page) pollImportReceipt(kind, requestID string) {
	if requestID == "" || p.closed {
		return
	}
	method := kind + ".status"
	oneBotCall(p, method, map[string]string{"requestId": requestID}, func(receipt *ImportResult, err error) {
		if err != nil {
			if kind == "market" {
				p.market.error = err.Error()
			} else {
				p.importing.error = err.Error()
			}
			return
		}
		if receipt == nil {
			return
		}
		if kind == "market" {
			p.market.receipt = receipt
		} else {
			p.importing.receipt = receipt
		}
		if receipt.Status == "running" {
			cancel := p.after(time.Second, func() { p.pollImportReceipt(kind, requestID) })
			if kind == "market" {
				p.market.pollCancel = cancel
			} else {
				p.importing.pollCancel = cancel
			}
		} else {
			p.refresh()
		}
	})
}
func (p *page) importView(c *ui.Context) {
	s := &p.importing
	if !s.open {
		return
	}
	open := true
	kit.Dialog(c, &open, kit.DialogOptions{Title: nt("import"), Width: 760, Cancel: nt("close"), Error: s.error, Busy: s.busy, Body: func() {
		if s.preview == nil {
			for _, source := range s.sources {
				if kit.Button(c, source.Name, icons.Download, false).FillWidth().Justify(ui.Start).Clicked() {
					p.previewImport(source)
				}
			}
			if len(s.sources) == 0 {
				kit.Help(c, nt("noImportSources"))
			}
			return
		}
		ui.Field(c, nt("name"), func() { ui.TextInput(c, &s.name).FillWidth().Label(nt("name")) })
		kit.Help(c, s.preview.Source.Name)
		for _, category := range []string{"personality", "memory", "skills", "bots", "connections", "automations"} {
			entries := []ImportEntry{}
			ids := []string{}
			for _, entry := range s.preview.Entries {
				if entry.Category == category {
					entries = append(entries, entry)
					ids = append(ids, entry.ID)
				}
			}
			if len(entries) == 0 {
				continue
			}
			kit.Heading(c, nt("importCategory."+category))
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				if kit.GhostButton(c, nt("selectAll"), nil, false).Clicked() {
					s.selected = toggleImportEntries(s.preview.Entries, s.selected, ids, true)
				}
				if kit.GhostButton(c, nt("selectNone"), nil, false).Clicked() {
					s.selected = toggleImportEntries(s.preview.Entries, s.selected, ids, false)
				}
			})
			for _, entry := range entries {
				checked := slices.Contains(s.selected, entry.ID)
				if kit.Toggle(c, &checked, entry.Name, entry.Description).Changed() {
					s.selected = toggleImportEntries(s.preview.Entries, s.selected, []string{entry.ID}, checked)
				}
				for _, issue := range entry.Issues {
					kit.Notice(c, kit.Caution, "", nt("importIssue", "code", issue), nil)
				}
				for _, dependency := range entry.DependsOn {
					if checked && !slices.Contains(s.selected, dependency) {
						kit.Notice(c, kit.Caution, "", nt("missingImportDependency", "id", dependency), nil)
					}
				}
			}
		}
		kit.Toggle(c, &s.takeover, nt("takeover"), nt("takeoverHint"))
		if kit.PrimaryButton(c, nt("startImport"), icons.Download, s.busy).Disabled(s.busy || strings.TrimSpace(s.name) == "").Clicked() {
			p.submitImport()
		}
		p.importReceiptView(c, s.receipt, s.preview)
	}})
	if !open {
		p.env.RequestChange(func() { s.open = false })
	}
}
func (p *page) importReceiptView(c *ui.Context, receipt *ImportResult, preview *ImportPreview) {
	if receipt == nil {
		return
	}
	kit.Strong(c, nt("importStatus."+receipt.Status))
	for _, check := range receipt.Checks {
		name := check.EntryID
		if preview != nil {
			for _, entry := range preview.Entries {
				if entry.ID == check.EntryID {
					name = entry.Name
				}
			}
		}
		text := name + " · " + nt("importCheck."+check.Status)
		if check.Progress != nil {
			text += fmt.Sprintf(" (%d/%d)", check.Progress.Saved, check.Progress.Total)
		}
		kit.Status(c, text)
		if check.Message != "" {
			kit.Status(c, check.Message)
		}
	}
	if receipt.BotID != "" && receipt.Saved && kit.Button(c, nt("openCompanion"), icons.ArrowUpRight, false).Clicked() {
		if receipt.CanonicalSessionID != "" {
			p.openSession(receipt.BotID, receipt.CanonicalSessionID)
		} else {
			p.openSettings(receipt.BotID, "home")
		}
	}
	if receipt.Status == "needs-attention" && kit.Button(c, nt("retry"), icons.RefreshCw, false).Clicked() {
		kind := "import"
		if preview == nil {
			kind = "market"
		}
		p.pollImportReceipt(kind, receipt.RequestID)
	}
}
