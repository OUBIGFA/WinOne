package studybook

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"strings"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

func (b *Book) bulkActions(c *ui.Context) {
	var noteIDs, cardIDs []string
	for _, n := range b.notes {
		if b.selected[n.ID] {
			noteIDs = append(noteIDs, n.ID)
			cardIDs = append(cardIDs, n.CardIDs...)
		}
	}
	if len(noteIDs) == 0 {
		return
	}
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		for _, key := range []string{"move", "addTags", "removeTags", "suspend", "resume", "flag", "due", "reset", "deleteNotes"} {
			key := key
			if kit.GhostButton(c, text(key), nil, false).Clicked() {
				method := ""
				params := map[string]any{}
				switch key {
				case "move":
					method = "cards.move"
					params = map[string]any{"ids": cardIDs, "deck": map[string]string{"id": b.deckID}}
				case "addTags", "removeTags":
					method = "tags.add"
					if key == "removeTags" {
						method = "tags.remove"
					}
					params = map[string]any{"noteIds": noteIDs, "tags": []any{}}
				case "suspend", "resume":
					method = "cards.setSuspended"
					params = map[string]any{"ids": cardIDs, "suspended": key == "suspend"}
				case "flag":
					method = "cards.setFlags"
					params = map[string]any{"ids": cardIDs, "flag": float64(1)}
				case "due":
					method = "scheduling.setDueDate"
					params = map[string]any{"cardIds": cardIDs, "days": "0"}
				case "reset":
					method = "scheduling.reset"
					params = map[string]any{"cardIds": cardIDs}
				case "deleteNotes":
					method = "notes.delete"
					params = map[string]any{"ids": noteIDs}
				}
				b.openTool(method, params)
			}
		}
	})
}
func (b *Book) fileTools(c *ui.Context) {
	kit.Heading(c, text("files"))
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		kit.TextInput(c, &b.filePath).Grow(1).Basis(220).MinWidth(0).Label(text("filePath")).Placeholder(text("filePath"))
		if kit.Button(c, text("chooseFile"), icons.FolderOpen, false).Clicked() {
			core.Go(b.env, func() (string, error) {
				return b.env.Dialogs.PickFile(text("import"), core.Filter{Name: text("ankiFiles"), Extensions: []string{"apkg", "colpkg", "csv", "tsv", "txt", "json", "db"}})
			}, func(path string, err error) {
				if b.closed {
					return
				}
				if err != nil {
					b.err = err.Error()
				} else if path != "" {
					b.filePath = path
				}
			})
		}
		if kit.Button(c, text("previewImport"), icons.Eye, false).Disabled(b.filePath == "").Clicked() {
			b.act("winone.import.preview", map[string]string{"path": b.filePath}, func(raw json.RawMessage) {
				var result any
				if err := json.Unmarshal(raw, &result); err != nil {
					b.err = err.Error()
					return
				}
				b.filePreview = result
				if value, ok := result.(map[string]any); ok {
					if metadata, ok := value["metadata"].(map[string]any); ok {
						b.importPath = b.filePath
						b.importForm = toolForm{value: metadata, inputs: map[string]string{}, variants: map[string]string{}, errors: map[string]string{}}
						b.act("winone.import.schema", map[string]any{"path": b.filePath}, func(raw json.RawMessage) {
							var schema map[string]any
							if err := json.Unmarshal(raw, &schema); err != nil {
								b.err = err.Error()
								return
							}
							b.importForm.catalog = &toolCatalog{Definitions: map[string]any{}, Actions: []toolAction{{ID: "csv", Schema: schema}}}
						})
					}
				}
			})
		}
		if kit.Button(c, text("import"), icons.Download, false).Disabled(b.filePath == "" || strings.HasSuffix(strings.ToLower(b.filePath), ".colpkg") && !b.replaceConfirmed).Clicked() {
			params := map[string]any{"path": b.filePath, "replaceConfirmed": b.replaceConfirmed}
			if b.importPath == b.filePath && b.importForm.catalog != nil {
				if len(b.importForm.errors) > 0 {
					b.err = text("invalidNumber")
					return
				}
				params["metadata"] = b.importForm.value
			}
			b.act("winone.import.run", params, func(raw json.RawMessage) {
				b.notice = text("completed")
				b.replaceConfirmed = false
				b.importForm = toolForm{}
				b.filePreview = nil
				b.load()
			})
		}
	})
	if strings.HasSuffix(strings.ToLower(b.filePath), ".colpkg") {
		ui.Checkbox(c, &b.replaceConfirmed, text("confirmReplace"))
	}
	if b.importPath == b.filePath && b.importForm.catalog != nil {
		f := &b.importForm
		f.field(c, text("importMapping"), "csv", f.catalog.Actions[0].Schema, f.value, func(value any) { f.value, _ = value.(map[string]any); f.dirty = true })
	} else if b.filePreview != nil {
		renderValue(c, "", b.filePreview)
	}
	if kit.GhostButton(c, text("cancelImport"), icons.X, false).Clicked() {
		b.act("winone.import.cancel", map[string]any{}, func(json.RawMessage) { b.filePath = ""; b.filePreview = nil; b.importForm = toolForm{} })
	}
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		if b.exportKind == "" {
			b.exportKind = ".apkg"
			b.includeMedia = true
			b.includeScheduling = true
		}
		kit.Select(c, &b.exportKind, []kit.SelectOption{{Value: ".apkg", Label: text("deckPackage")}, {Value: ".colpkg", Label: text("collectionPackage")}, {Value: ".txt", Label: text("textFile")}})
		ui.Checkbox(c, &b.includeMedia, text("includeMedia"))
		ui.Checkbox(c, &b.includeScheduling, text("includeScheduling"))
		if kit.Button(c, text("export"), icons.Upload, false).Clicked() {
			kind, media, schedule := b.exportKind, b.includeMedia, b.includeScheduling
			core.Go(b.env, func() (string, error) {
				return b.env.Dialogs.SaveFile(text("export"), core.Filter{Name: text("ankiFiles"), Extensions: []string{strings.TrimPrefix(kind, ".")}}, "WinOne"+kind)
			}, func(path string, err error) {
				if b.closed {
					return
				}
				if err != nil {
					b.err = err.Error()
					return
				}
				if path != "" {
					b.act("winone.export", map[string]any{"path": path, "kind": kind, "includeMedia": media, "includeScheduling": schedule}, func(json.RawMessage) { b.notice = text("exported") })
				}
			})
		}
	})
}
func (b *Book) profileTools(c *ui.Context) {
	kit.Heading(c, text("profiles"))
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		current := b.state.ActiveProfile
		var options []kit.SelectOption
		for _, name := range b.state.Profiles {
			options = append(options, kit.SelectOption{Value: name, Label: name})
		}
		if changed, _ := kit.Select(c, &current, options); changed {
			b.act("profiles.switch", map[string]string{"name": current}, func(json.RawMessage) { b.deckID = ""; b.modelID = ""; b.notes = nil; b.review = nil; b.load() })
		}
		kit.TextInput(c, &b.newProfile).Grow(1).Basis(180).MinWidth(0).Label(text("profileName")).Placeholder(text("profileName"))
		if kit.Button(c, text("createProfile"), icons.Plus, false).Disabled(strings.TrimSpace(b.newProfile) == "").Clicked() {
			b.act("winone.profiles.create", map[string]string{"name": b.newProfile}, func(json.RawMessage) { b.newProfile = ""; b.load() })
		}
	})
}

func (b *Book) syncTools(c *ui.Context) {
	kit.Heading(c, text("sync"))
	ui.Field(c, text("fullSync"), func() {
		kit.Select(c, &b.fullSyncDirection, []kit.SelectOption{{Value: "", Label: text("normalSync")}, {Value: "upload", Label: text("uploadCollection")}, {Value: "download", Label: text("downloadCollection")}})
	}).FillWidth()
	if b.fullSyncDirection != "" {
		ui.Checkbox(c, &b.fullSyncConfirmed, text("confirmFullSync"))
		if kit.Button(c, text("runFullSync"), icons.RefreshCw, false).Disabled(!b.fullSyncConfirmed).Clicked() {
			b.act("winone.sync.run", map[string]any{"direction": b.fullSyncDirection, "confirmed": b.fullSyncConfirmed, "includeMedia": true}, func(json.RawMessage) {
				b.fullSyncConfirmed = false
				b.fullSyncDirection = ""
				b.notice = text("synced")
				b.load()
			})
		}
	}
	kit.Help(c, text("syncHint"))
	ui.Field(c, text("username"), func() { kit.TextInput(c, &b.username).FillWidth() }).FillWidth()
	ui.Field(c, text("password"), func() { kit.TextInput(c, &b.password).Password().FillWidth() }).FillWidth()
	ui.Field(c, text("endpoint"), func() { kit.TextInput(c, &b.endpoint).FillWidth() }).FillWidth()
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		if kit.Button(c, text("signIn"), icons.UserPlus, false).Disabled(b.username == "" || b.password == "").Clicked() {
			b.act("winone.sync.login", map[string]string{"username": b.username, "password": b.password, "endpoint": b.endpoint}, func(json.RawMessage) { b.password = ""; b.notice = text("signedIn") })
		}
		if kit.Button(c, text("syncNow"), icons.RefreshCw, false).Clicked() {
			b.act("winone.sync.run", map[string]any{"includeMedia": true}, func(json.RawMessage) { b.notice = text("synced"); b.load() })
		}
		if kit.GhostButton(c, text("signOut"), icons.LogOut, false).Clicked() {
			b.act("winone.sync.logout", map[string]any{}, func(json.RawMessage) { b.password = ""; b.notice = text("signedOut") })
		}
	})
}
