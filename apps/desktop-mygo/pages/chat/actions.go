package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/webui"
	"winone/desktopmygo/widgets"
)

// maxAttachments is how many files a draft carries.
const maxAttachments = 16

// addPaths attaches files and references folders, as @path lines: the
// files are copied into the conversation (kept as paths for a chat not
// created yet).
func (p *Chat) addPaths(chosen []string, done func(error)) {
	id := p.activeID
	var files, folders []string
	for _, path := range chosen {
		info, err := os.Stat(path)
		if err != nil {
			done(err)
			return
		}
		if info.IsDir() {
			folders = append(folders, path)
		} else {
			files = append(files, path)
		}
	}
	if len(p.files)+len(files) > maxAttachments {
		done(core.Errorf(i18n.T("chat.attachmentCountLimit")))
		return
	}
	attach := func(added []model.Attachment) {
		if p.selection != id {
			done(core.Errorf(i18n.T("chat.browserDraftBusy")))
			return
		}
		var references []string
		for _, folder := range folders {
			references = append(references, "@"+folder)
		}
		text := joinNonEmpty("\n", p.text, strings.Join(references, "\n"))
		p.updateText(text, mergeFiles(p.files, added), id)
		done(nil)
	}
	if len(files) == 0 {
		attach(nil)
		return
	}
	if id == "" {
		var added []model.Attachment
		for _, path := range files {
			added = append(added, model.Attachment{Name: filepath.Base(path), Path: path})
		}
		attach(added)
		return
	}
	chatCall(p, "attachments.add", map[string]any{"id": id, "paths": files}, func(r struct {
		Attachments []model.Attachment `json:"attachments"`
	}, err error) {
		if err != nil {
			done(err)
			return
		}
		attach(r.Attachments)
	})
}

// pickFiles chooses files to attach.
func (p *Chat) pickFiles(done func(error)) {
	core.Go(p.env, func() ([]string, error) {
		return p.env.Dialogs.PickFiles(i18n.T("chat.uploadAttachment"), core.Filter{})
	},
		func(chosen []string, err error) {
			if err != nil || len(chosen) == 0 {
				done(err)
				return
			}
			p.addPaths(chosen, done)
		})
}

// configure changes the model or permission. A conversation of a project
// takes the project's permission; the new chat keeps its choice until it
// is created; an open conversation reconnects to its restarted runtime.
func (p *Chat) configure(selection *widgets.AISelection, permission model.Permission, done func(error)) {
	if selection != nil {
		p.prefs.LastModel = selection
		p.prefs.save()
	}
	conversation := p.conversation()
	projectID := p.newProjectID
	if conversation != nil {
		projectID = conversation.ProjectID
	}
	if permission != "" && projectID != "" {
		p.afterWrites(func() {
			chatCall(p, "project.permission", map[string]any{"id": projectID, "permission": permission}, func(updated Catalog, err error) {
				if err != nil {
					done(err)
					return
				}
				p.catalog, p.newPermission = updated, permission
				if conversation != nil {
					p.reconnect()
				}
				done(nil)
			})
		})
		return
	}
	if conversation == nil {
		if selection != nil {
			p.newModel = *selection
		}
		if permission != "" {
			p.newPermission = permission
		}
		done(nil)
		return
	}
	id := conversation.ID
	params := map[string]any{"id": id}
	if selection != nil {
		params["model"] = selection
	}
	if permission != "" {
		params["permission"] = permission
	}
	p.afterWrites(func() {
		p.configuring, p.reconnecting = id, true
		chatCall(p, "configure", params, func(updated Conversation, err error) {
			if err != nil {
				p.configuring, p.reconnecting = "", false
				done(err)
				return
			}
			p.catalog.replace(updated)
			if p.activeID == id {
				p.reconnect()
			}
			done(nil)
		})
	})
}

// setThinking and setFast change the conversation's thinking level and
// fast mode, or the new chat's.
func (p *Chat) setThinking(level string, done func(error)) {
	c := p.conversation()
	if c == nil {
		p.newThinking = level
		done(nil)
		return
	}
	chatCall(p, "thinking", map[string]any{"id": c.ID, "thinkingLevel": level}, func(updated Conversation, err error) {
		if err == nil {
			p.catalog.replace(updated)
		}
		done(err)
	})
}

func (p *Chat) setFast(on bool, done func(error)) {
	c := p.conversation()
	if c == nil {
		p.newFastMode = on
		done(nil)
		return
	}
	chatCall(p, "fast", map[string]any{"id": c.ID, "fastMode": on}, func(updated Conversation, err error) {
		if err == nil {
			p.catalog.replace(updated)
		}
		done(err)
	})
}

// updateConversation renames, pins or archives a conversation; archiving
// the open one leaves it for a new chat.
func (p *Chat) updateConversation(target Conversation, change map[string]any, done func(error)) {
	params := map[string]any{"id": target.ID}
	for k, v := range change {
		params[k] = v
	}
	chatCall(p, "conversation.update", params, func(updated Conversation, err error) {
		if err != nil {
			done(err)
			return
		}
		if _, ok := change["title"]; ok {
			p.titleEdits[target.ID]++
		}
		p.catalog.replace(updated)
		if archived, _ := change["archived"].(bool); archived && target.ID == p.activeID {
			p.choose("", nil, done)
			return
		}
		done(nil)
	})
}

// generateTitle asks the Host for an AI title in the background: from
// the first message (text) or the saved branch (nil). A manual rename
// made meanwhile wins; on failure the Host's saved fallback is read once.
func (p *Chat) generateTitle(target Conversation, text *string) {
	if p.titleRequests[target.ID] {
		return
	}
	revision := p.titleEdits[target.ID]
	p.titleRequests[target.ID] = true
	p.titleJobs[target.ID] = titleJob{pending: true}
	method, params := "title.regenerate", map[string]any{"id": target.ID}
	if text != nil {
		method, params["text"] = "title.generate", *text
	}
	current := func() bool { return p.titleEdits[target.ID] == revision }
	apply := func(title string) {
		if c := p.catalog.conversation(target.ID); c != nil && current() && c.Title == target.Title {
			c.Title = title
		}
	}
	chatCall(p, method, params, func(updated Conversation, err error) {
		if err == nil {
			apply(updated.Title)
			p.titleJobs[target.ID] = titleJob{}
			delete(p.titleRequests, target.ID)
			return
		}
		chatCall(p, "catalog", map[string]any{}, func(saved Catalog, readErr error) {
			if readErr != nil {
				p.err = readErr.Error()
			} else if c := saved.conversation(target.ID); c != nil {
				apply(c.Title)
			}
			job := titleJob{}
			if current() {
				job.err = err.Error()
			}
			p.titleJobs[target.ID] = job
			delete(p.titleRequests, target.ID)
		})
	})
}

// moveConversation moves a conversation to a project; the open one
// reconnects to the runtime the Host restarts for it.
func (p *Chat) moveConversation(id, projectID string, done func(error)) {
	p.afterWrites(func() {
		reconnect := id == p.activeID
		if reconnect {
			p.configuring, p.reconnecting = id, true
		}
		chatCall(p, "conversation.move", map[string]any{"id": id, "projectId": projectID}, func(updated Conversation, err error) {
			if err != nil {
				if reconnect {
					p.configuring, p.silentReload, p.reconnecting = "", "", false
				}
				done(err)
				return
			}
			p.catalog.replace(updated)
			if reconnect {
				p.reconnect()
			}
			project := i18n.T("chat.noProject")
			if pr := p.catalog.project(projectID); pr != nil {
				project = pr.Name
			}
			p.notice = i18n.T("chat.moveDone", "name", updated.Title, "project", project)
			done(nil)
		})
	})
}

// fork copies the conversation (from entry when given) and opens it.
func (p *Chat) fork(entry string, done func(error)) {
	if p.activeID == "" {
		done(nil)
		return
	}
	params := map[string]any{"id": p.activeID}
	if entry != "" {
		params["entry"] = entry
	}
	chatCall(p, "fork", params, func(created Conversation, err error) {
		if err != nil {
			done(err)
			return
		}
		p.refresh(func(err error) {
			if err != nil {
				done(err)
				return
			}
			p.choose(created.ID, nil, done)
		})
	})
}

// deleteConversation deletes a conversation; deleting the open one
// leaves it for a new chat.
func (p *Chat) deleteConversation(id string, done func(error)) {
	p.afterWrites(func() {
		chatCall(p, "conversation.delete", map[string]any{"id": id}, func(updated Catalog, err error) {
			if err != nil {
				done(err)
				return
			}
			p.catalog = updated
			if p.activeID == id {
				p.choose("", nil, done)
				return
			}
			done(nil)
		})
	})
}

// saveProject opens a folder as a project, or creates one in parent, and
// starts a new chat in it.
func (p *Chat) saveProject(create bool, name, folder string, permission model.Permission, done func(error)) {
	method, params := "project.add", map[string]any{"path": folder, "name": strings.TrimSpace(name), "permission": permission}
	if create {
		method, params = "project.create", map[string]any{"parent": folder, "name": strings.TrimSpace(name), "permission": permission}
	}
	chatCall(p, method, params, func(project Project, err error) {
		if err != nil {
			done(err)
			return
		}
		p.refresh(func(err error) {
			if err != nil {
				done(err)
				return
			}
			p.choose("", nil, func(err error) {
				if err == nil {
					p.newProjectID, p.newPermission = project.ID, permission
					p.ui.automations = false
				}
				done(err)
			})
		})
	})
}

// changeProject renames (name non-nil) or removes a project.
func (p *Chat) changeProject(id string, name *string, done func(error)) {
	method, params := "project.remove", map[string]any{"id": id}
	if name != nil {
		method, params["name"] = "project.update", *name
	}
	p.simple(method, params, func(err error) {
		if err != nil {
			done(err)
			return
		}
		if name == nil && p.newProjectID == id {
			p.newProjectID = ""
		}
		p.refresh(done)
	})
}

// saveLocations changes the open conversation's save folder, or the new
// chat's working and save folders.
func (p *Chat) saveLocations(working, saved string, done func(error)) {
	c := p.conversation()
	if c == nil {
		p.location, p.saveLocation = working, saved
		done(nil)
		return
	}
	if saved == c.SessionDirectory {
		done(nil)
		return
	}
	chatCall(p, "location", map[string]any{"id": c.ID, "saveDirectory": saved}, func(updated Conversation, err error) {
		if err != nil {
			done(err)
			return
		}
		p.catalog.replace(updated)
		p.open(false)
		p.notice = i18n.T("chat.locationChanged")
		done(nil)
	})
}

// exportOutcome is chat.export's result.
type exportOutcome struct {
	Status          string   `json:"status"`
	TotalBytes      float64  `json:"totalBytes"`
	MediaBytes      float64  `json:"mediaBytes"`
	LimitBytes      float64  `json:"limitBytes"`
	MessagesBytes   *float64 `json:"messagesBytes"`
	TranscriptBytes *float64 `json:"transcriptBytes"`
	FilePath        string   `json:"filePath"`
	Fidelity        string   `json:"fidelity"`
	UnpackedBytes   float64  `json:"unpackedBytes"`
	FileBytes       float64  `json:"fileBytes"`
	MediaMissing    float64  `json:"mediaMissing"`
	MediaDropped    float64  `json:"mediaDropped"`
	MediaExcluded   float64  `json:"mediaExcluded"`
	MissingFiles    []string `json:"missingTranscripts"`
	Workers         float64  `json:"orcaWorkers"`
	MessageCount    float64  `json:"messageCount"`
}

// export saves a conversation as a Cindy .cshare archive. Only this
// window chooses the destination, as the React window's native bridge
// did (apps/desktop-rust/src/chat_export.rs); the Host's receipt must name
// that file, or report the size it exceeds.
func (p *Chat) export(id, password string, excludeMedia bool, done func(exportOutcome, error)) {
	params := map[string]any{"id": id}
	if password != "" {
		params["password"] = password
	}
	if excludeMedia {
		params["excludeMedia"] = true
	}
	request, _ := json.Marshal(params)
	p.afterWrites(func() {
		core.Go(p.env, func() (exportOutcome, error) {
			var out exportOutcome
			receipt, err := webui.ExportTask(p.env, request)
			if err == nil {
				err = json.Unmarshal(receipt, &out)
			}
			return out, err
		}, done)
	})
}

// raw is a request whose result the page does not read.
type raw = json.RawMessage
