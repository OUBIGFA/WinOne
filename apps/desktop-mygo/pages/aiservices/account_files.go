package aiservices

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

type accountImportResult struct {
	User, Status, Error string
	File                int `json:"file,omitempty"`
	filename            string
}

type accountExportResult struct {
	Files   []string              `json:"files"`
	Skipped []accountImportResult `json:"skipped"`
}

// Account files are read only for an explicit import, sent once, then dropped.
// The native page never persists or displays their credential contents.
func (a *accountPanel) importView(c *ui.Context, request accountRequest, disabled bool) {
	p := a.p
	kit.Group(c, icons.FolderUp, nativeT("importAccounts"), a.importHint, func() {
		kit.Stack(c, "", "", func() {
			area := ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
				if kit.Button(c, nativeT("chooseAccountFiles"), icons.FolderOpen, false).Disabled(disabled).Clicked() {
					a.importBusy = true
					owner := a.owner
					core.Go(p.env, func() ([]string, error) { return p.env.Dialogs.PickFiles(nativeT("chooseAccountFiles"), core.Filter{}) }, func(paths []string, err error) {
						if owner != a.owner {
							return
						}
						a.importBusy = false
						if err != nil {
							a.err = err.Error()
						} else if len(paths) > 0 {
							a.importPaths, a.importResults = paths, nil
						}
					})
				}
			})
			widgets.DropArea(c, p.env, area, widgets.DropFiles, true, disabled, func(paths []string, err error) {
				if err != nil {
					a.err = err.Error()
				} else {
					a.importPaths, a.importResults = paths, nil
				}
			})
			for _, path := range a.importPaths {
				kit.Help(c, filepath.Base(path))
			}
			if request.Agent == "codex" {
				kit.Toggle(c, &a.accessOnly, nativeT("accessOnly"), nativeT("accessOnlyHint")).Disabled(disabled)
				if !a.accessOnly {
					kit.Help(c, nativeT("importRotates"))
				}
			}
			ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
				if kit.Button(c, nativeT("confirmImport"), icons.FolderUp, a.importBusy).Disabled(disabled || len(a.importPaths) == 0).Clicked() {
					a.importAccounts(request)
				}
				if kit.GhostButton(c, i18n.T("common.cancel"), nil, false).Disabled(disabled || len(a.importPaths) == 0).Clicked() {
					a.importPaths = nil
				}
			})
			for _, result := range a.importResults {
				label := nativeT("import." + result.Status)
				if result.Error != "" {
					label = nativeT("import.failed")
				}
				name := result.User
				if name == "" {
					name = result.filename
				}
				kit.Help(c, name+" · "+label).Role(ui.RoleStatus)
			}
		})
	})
}

func (a *accountPanel) importAccounts(request accountRequest) {
	p, owner := a.p, a.owner
	paths := append([]string(nil), a.importPaths...)
	a.importBusy, a.err = true, ""
	params := map[string]any{"agent": request.Agent, "provider": request.Provider, "accessOnly": a.accessOnly}
	if request.Draft != nil {
		params["draft"] = request.Draft
	}
	core.Go(p.env, func() ([]accountImportResult, error) {
		var texts []string
		total := 0
		for _, path := range paths {
			file, err := os.Open(path)
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
			file.Close()
			if err != nil {
				return nil, err
			}
			total += len(data)
			if total > 8<<20 {
				return nil, errors.New(nativeT("accountFilesTooLarge"))
			}
			texts = append(texts, string(data))
		}
		params["files"] = texts
		var out struct {
			Results []accountImportResult `json:"results"`
		}
		err := p.env.Host.CallCompletingInto("capture.ai.accounts.import", params, &out)
		return out.Results, err
	}, func(results []accountImportResult, err error) {
		if owner != a.owner {
			return
		}
		a.importBusy = false
		if err != nil {
			a.err = err.Error()
			return
		}
		// An unreadable account has no user name. Keep the upstream file index
		// useful after the pending selection is cleared, retaining only its name.
		for i := range results {
			if file := results[i].File; results[i].User == "" && file > 0 && file <= len(paths) {
				results[i].filename = filepath.Base(paths[file-1])
			}
		}
		a.importPaths, a.importResults = nil, results
		p.reader.Invalidate()
		a.load(request, true)
	})
}

func (a *accountPanel) exportView(c *ui.Context, request accountRequest, disabled bool) {
	kit.Group(c, icons.Save, nativeT("exportAccounts"), nativeT("exportHint"), func() {
		kit.Stack(c, "", "", func() {
			if a.exportFormat == "" {
				a.exportFormat = "sub2api"
			}
			nativeSelect(c, nativeT("exportFormat"), &a.exportFormat, options("sub2api", "Sub2API", "cliproxyapi", "CLIProxyAPI"), disabled)
			if a.exportUsers == nil {
				a.exportUsers = map[string]bool{}
				for _, account := range a.accounts {
					a.exportUsers[account.User] = true
				}
			}
			var users []string
			for _, account := range a.accounts {
				on := a.exportUsers[account.User]
				if ui.Checkbox(c.Key(account.User), &on, account.User).Disabled(disabled).Changed() {
					a.exportUsers[account.User] = on
				}
				if on {
					users = append(users, account.User)
				}
			}
			if kit.Button(c, nativeT("chooseExportPath"), icons.Save, a.importBusy).Disabled(disabled || len(users) == 0).Clicked() {
				p, owner, format := a.p, a.owner, a.exportFormat
				users := append([]string(nil), users...)
				a.importBusy, a.err = true, ""
				core.Go(p.env, func() (accountExportResult, error) {
					var path string
					var err error
					if format == "sub2api" {
						path, err = p.env.Dialogs.SaveFile(nativeT("exportAccounts"), core.Filter{Name: "JSON", Extensions: []string{"json"}}, "WinOne-sub2api-"+request.Agent+".json")
					} else {
						path, err = p.env.Dialogs.PickFolder(nativeT("chooseExportPath"))
					}
					if err != nil || path == "" {
						return accountExportResult{}, err
					}
					params := map[string]any{"agent": request.Agent, "provider": request.Provider, "format": format, "path": path, "users": users}
					if request.Draft != nil {
						params["draft"] = request.Draft
					}
					var out accountExportResult
					err = p.env.Host.CallCompletingInto("capture.ai.accounts.export", params, &out)
					return out, err
				}, func(out accountExportResult, err error) {
					if owner != a.owner {
						return
					}
					a.importBusy = false
					if err != nil {
						a.err = err.Error()
					} else {
						a.exportFiles, a.exportSkipped = out.Files, out.Skipped
					}
				})
			}
			for _, path := range a.exportFiles {
				kit.Help(c, path).Selectable()
			}
			for _, skipped := range a.exportSkipped {
				kit.Help(c, skipped.User+" · "+nativeT("exportSkipped")).Role(ui.RoleStatus)
			}
		})
	})
}
