package aiservices

import (
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo/ui"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

var libraryTabs = []string{"Instructions", "MCP servers", "Skills", "Hooks", "Subagents", "RTK"}
var libraryKinds = []string{"instructions", "servers", "skills", "hooks", "subagents", "rtk"}

func (x *externalPanel) libraryView(c *ui.Context) {
	if x.library == nil {
		return
	}
	labels := make([]string, len(libraryTabs))
	for i, v := range libraryTabs {
		labels[i] = upstreamT(v)
	}
	old := x.tab
	kit.SubTabs(c, &x.tab, labels, x.p.env.RequestChange)
	if old != x.tab {
		clear(x.selected)
		x.query = ""
	}
	if x.library["problems"] != nil {
		externalDetails(c, x.library["problems"])
	}
	if x.edit != nil {
		x.editorView(c)
		return
	}
	if x.skill != nil {
		x.skillView(c)
		return
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		ui.TextInput(c, &x.query).Width(260).Label(upstreamT("Search"))
		if kit.Button(c, upstreamT("Sync"), icons.RefreshCw, false).Disabled(x.working).Clicked() {
			x.ask(upstreamT("Sync"), "/api/library/all/sync", map[string]any{}, x.acceptLibrary)
		}
		for _, key := range []string{"dir", "backups"} {
			if kit.Button(c, upstreamT(map[string]string{"dir": "Library folder", "backups": "Backups"}[key]), icons.FolderOpen, false).Disabled(x.working).Clicked() {
				x.request("POST", "/api/library/reveal", map[string]string{"path": textOf(x.library, key)}, func(json.RawMessage) {})
			}
		}
	})
	kind := libraryKinds[x.tab]
	switch kind {
	case "instructions":
		x.instructionsView(c)
	case "rtk":
		x.rtkView(c)
	default:
		x.itemsView(c, kind)
	}
	if kind == "skills" {
		kit.Strong(c, upstreamT("Projects"))
		x.itemsView(c, "projects")
	}
	if kind == "servers" || kind == "skills" {
		if x.marketKind != kind {
			x.market = nil
			x.marketQuery = ""
		}
		x.marketKind = kind
		kit.Strong(c, upstreamT("Discover"))
		x.marketView(c)
	}
	if x.result != nil {
		kit.Strong(c, upstreamT("Result"))
		externalDetails(c, x.result)
		if kit.Button(c, i18n.T("common.close"), nil, false).Clicked() {
			x.result = nil
		}
	}
}
func (x *externalPanel) agentsView(c *ui.Context, chosen map[string]bool) {
	kit.Strong(c, externalT("agents"))
	ui.Row(c).Gap(8).Children(func() {
		if kit.Button(c, externalT("all"), nil, false).Disabled(x.working).Clicked() {
			for _, a := range rowsOf(x.library, "agents") {
				if !boolOf(a, "gone") && !boolOf(a, "unseen") {
					chosen[textOf(a, "id")] = true
				}
			}
		}
		if kit.Button(c, externalT("none"), nil, false).Disabled(x.working).Clicked() {
			clear(chosen)
		}
	})
	ui.Row(c).FillWidth().Wrap().Gap(12).Children(func() {
		for _, a := range rowsOf(x.library, "agents") {
			id := textOf(a, "id")
			on := chosen[id]
			label := textOf(a, "name")
			if note := textOf(a, "note"); note != "" {
				label += " · " + note
			}
			if ui.Checkbox(c.Key(id), &on, label).Disabled(x.working || boolOf(a, "unseen") || boolOf(a, "gone") && !on).Changed() {
				chosen[id] = on
			}
		}
	})
}
func (x *externalPanel) begin(kind string, item map[string]any) {
	original := map[string]any{}
	for k, v := range item {
		original[k] = v
	}
	values := map[string]string{}
	for k, v := range item {
		switch t := v.(type) {
		case string:
			values[k] = t
		case float64:
			values[k] = strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			values[k] = strconv.FormatBool(t)
		case []any:
			values[k] = strings.Join(stringsOf(t), "\n")
		case map[string]any:
			values[k] = mapText(t)
		}
	}
	x.edit = &libraryEditor{kind: kind, old: textOf(item, "name"), values: values, original: original, agents: selectedStrings(stringsOf(item["agents"])), chosen: map[string][]string{}}
	if kind == "servers" {
		x.edit.rememberServerFields(item)
	}
	if kind == "servers" && values["transport"] == "" {
		values["transport"] = "stdio"
	}
	if (kind == "servers" || kind == "servers-import") && item == nil {
		for _, a := range rowsOf(x.library, "agents") {
			if (textOf(a, "mcp") != "" || boolOf(a, "mcp")) && !boolOf(a, "gone") && !boolOf(a, "unseen") {
				x.edit.agents[textOf(a, "id")] = true
			}
		}
	}
}
func (x *externalPanel) instructionsView(c *ui.Context) {
	instructions := objectOf(x.library, "instructions")
	if kit.PrimaryButton(c, upstreamT("New set"), icons.Plus, false).Disabled(x.working).Clicked() {
		x.begin("instruction-create", map[string]any{"id": nonce()})
	}
	for _, set := range rowsOf(instructions, "sets") {
		id := textOf(set, "id")
		label := textOf(set, "name")
		if label == "" {
			label = upstreamT("Default")
		}
		if boolOf(set, "active") {
			label += " · " + upstreamT("Active")
		}
		kit.Section(c.Key(id), nil, label, textOf(set, "file"), func() {
			text := textOf(set, "text")
			ui.TextArea(c, &text).FillWidth().MinHeight(120).ReadOnly(true).Label(label)
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if kit.Button(c, upstreamT("Edit"), icons.Pencil, false).Disabled(x.working).Clicked() {
					x.begin("instructions", set)
				}
				if kit.Button(c, upstreamT("Use"), nil, false).Disabled(x.working || boolOf(set, "active")).Clicked() {
					x.act("instructions/save", map[string]any{"activate": id})
				}
				if kit.Button(c, upstreamT("Rename"), nil, false).Disabled(x.working).Clicked() {
					x.begin("instruction-rename", set)
				}
				if kit.Button(c, upstreamT("Remove"), icons.Trash2, false).Disabled(x.working || id == "default").Clicked() {
					x.ask(upstreamT("Remove"), "/api/library/instructions/save", map[string]string{"remove": id}, x.acceptLibrary)
				}
			})
		})
	}
	for _, agent := range rowsOf(instructions, "agents") {
		id := textOf(agent, "agent")
		on := boolOf(agent, "on")
		kit.Section(c.Key(id), nil, textOf(agent, "name"), textOf(agent, "path"), func() {
			if ui.Checkbox(c, &on, upstreamT("Shared instructions")).Disabled(x.working).Changed() {
				agents := []string{}
				for _, a := range rowsOf(instructions, "agents") {
					if textOf(a, "agent") == id && on || textOf(a, "agent") != id && boolOf(a, "on") {
						agents = append(agents, textOf(a, "agent"))
					}
				}
				x.act("instructions/save", map[string]any{"agents": agents})
			}
			if kit.Button(c, upstreamT("Extra instructions"), icons.Pencil, false).Disabled(x.working).Clicked() {
				x.begin("instruction-extra", map[string]any{"agent": id, "text": textOf(agent, "extra")})
			}
			if kit.Button(c, upstreamT("Bring in"), nil, false).Disabled(x.working).Clicked() {
				x.act("instructions/import", map[string]string{"name": id})
			}
			if boolOf(agent, "edited") && kit.Button(c, upstreamT("Rewrite"), nil, false).Disabled(x.working).Clicked() {
				x.ask(upstreamT("Rewrite"), "/api/library/instructions/save", map[string]any{"rewrite": []string{id}}, x.acceptLibrary)
			}
			if textOf(agent, "override") != "" {
				kit.Help(c, upstreamT("Override")+": "+textOf(agent, "override"))
			}
		})
	}
}

func (x *externalPanel) itemsView(c *ui.Context, kind string) {
	rows := rowsOf(x.library, kind)
	ui.Row(c).Gap(8).Wrap().Children(func() {
		if kit.PrimaryButton(c, upstreamT("Add"), icons.Plus, false).Disabled(x.working).Clicked() {
			if kind == "skills" {
				x.begin("skills-install", nil)
			} else {
				x.begin(kind, nil)
			}
		}
		if kind == "servers" && kit.Button(c, upstreamT("Paste JSON"), icons.ClipboardPaste, false).Disabled(x.working).Clicked() {
			x.begin("servers-import", nil)
		}
		if kit.Button(c, externalT("all"), nil, false).Disabled(x.working).Clicked() {
			for _, item := range rows {
				x.selected[textOf(item, "name")] = true
			}
		}
		if kit.Button(c, externalT("none"), nil, false).Disabled(x.working).Clicked() {
			clear(x.selected)
		}
		if kit.Button(c, externalT("actions"), nil, false).Disabled(x.working).Clicked() {
			x.begin("actions-"+kind, nil)
		}
		if kind == "skills" {
			for _, action := range []string{"check", "regroup", "update-all"} {
				if kit.Button(c, upstreamT(map[string]string{"check": "Check for updates", "regroup": "Group again", "update-all": "Update all"}[action]), nil, false).Disabled(x.working).Clicked() {
					x.ask(upstreamT("Update"), "/api/library/skills/"+action, map[string]any{}, x.acceptLibrary)
				}
			}
		}
		if kind == "servers" && kit.Button(c, upstreamT("Check all"), nil, false).Disabled(x.working).Clicked() {
			x.request("POST", "/api/library/mcp/check", map[string]any{"fresh": true}, x.receiveResult)
		}
	})
	if kind == "servers" || kind == "skills" {
		x.libraryOptions(c, kind)
	}
	count := 0
	for _, item := range rows {
		name := textOf(item, "name")
		if !strings.Contains(strings.ToLower(name+" "+textOf(item, "description")+" "+textOf(item, "source")), strings.ToLower(x.query)) {
			continue
		}
		count++
		kit.Section(c.Key(kind+name), nil, name, textOf(item, "description"), func() {
			on := x.selected[name]
			if ui.Checkbox(c, &on, externalT("selected")).Changed() {
				x.selected[name] = on
			}
			for _, key := range []string{"source", "kind", "transport", "url", "command", "event", "matcher", "model", "dir", "note", "gone"} {
				if value := textOf(item, key); value != "" {
					kit.Help(c, upstreamT(key)+": "+value)
				}
			}
			if problems := item["problems"]; problems != nil {
				externalDetails(c, problems)
			}
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if kit.Button(c, upstreamT("Edit"), icons.Pencil, false).Disabled(x.working).Clicked() {
					x.begin(kind, item)
				}
				if kind != "projects" && kit.Button(c, externalT("agents"), nil, false).Disabled(x.working).Clicked() {
					x.begin("assign-"+kind, item)
				}
				if kind == "skills" {
					x.skillButtons(c, item)
				}
				if kind == "servers" {
					x.serverButtons(c, item)
				}
				if kind == "projects" {
					x.projectButtons(c, item)
				}
				if kit.Button(c, upstreamT("Remove"), icons.Trash2, false).Disabled(x.working).Clicked() {
					body := map[string]any{"name": name}
					if kind == "projects" {
						body = map[string]any{"dir": textOf(item, "dir"), "keep": true}
					}
					x.ask(upstreamT("Remove"), "/api/library/"+kind+"/remove", body, x.acceptLibrary)
				}
			})
		})
	}
	if count == 0 {
		kit.Help(c, externalT("empty"))
	}
	if kind == "servers" || kind == "skills" {
		x.foundView(c, kind)
	}
}
func (x *externalPanel) receiveResult(raw json.RawMessage) {
	if err := json.Unmarshal(raw, &x.result); err != nil {
		x.err = err.Error()
	}
}
func (x *externalPanel) serverButtons(c *ui.Context, item map[string]any) {
	name := textOf(item, "name")
	if kit.Button(c, upstreamT("Check"), nil, false).Disabled(x.working).Clicked() {
		x.request("POST", "/api/library/mcp/check", map[string]any{"names": []string{name}, "fresh": true}, x.receiveResult)
	}
	if kit.Button(c, upstreamT("Sign in"), nil, false).Disabled(x.working).Clicked() {
		x.request("POST", "/api/library/mcp-signin", map[string]string{"name": name}, func(raw json.RawMessage) {
			x.receiveResult(raw)
			var state map[string]any
			_ = json.Unmarshal(raw, &state)
			x.begin("mcp-signin", state)
		})
	}
	if kit.Button(c, upstreamT("Sign out"), nil, false).Disabled(x.working).Clicked() {
		x.ask(upstreamT("Sign out"), "/api/library/mcp-signout", map[string]string{"name": name}, x.acceptLibrary)
	}
	if kit.Button(c, upstreamT("Tools"), nil, false).Disabled(x.working).Clicked() {
		x.begin("server-tools", item)
	}
	if plugin := textOf(item, "plugin"); plugin != "" && kit.Button(c, upstreamT("Turn plugin tools off"), nil, false).Disabled(x.working).Clicked() {
		x.act("servers/plugin-on", map[string]any{"name": plugin, "on": false})
	}
}
func (x *externalPanel) skillButtons(c *ui.Context, item map[string]any) {
	name := textOf(item, "name")
	if kit.Button(c, upstreamT("Files"), icons.FileText, false).Disabled(x.working).Clicked() {
		x.readSkill(name, "")
	}
	if kit.Button(c, upstreamT("Update"), icons.RefreshCw, false).Disabled(x.working || textOf(item, "source") == "").Clicked() {
		x.ask(upstreamT("Update"), "/api/library/skills/update", map[string]any{"name": name, "replace": boolOf(item, "edited")}, x.acceptLibrary)
	}
	on := !boolOf(item, "offHere")
	if ui.Checkbox(c, &on, upstreamT("On this computer")).Disabled(x.working).Changed() {
		x.act("skills/here", map[string]any{"name": name, "on": on})
	}
	direct := textOf(item, "router") == "direct"
	if ui.Checkbox(c, &direct, upstreamT("Give directly")).Disabled(x.working).Changed() {
		x.act("skills/router-direct", map[string]any{"name": name, "on": direct})
	}
	if kit.Button(c, upstreamT("Note"), nil, false).Disabled(x.working).Clicked() {
		x.begin("skill-note", item)
	}
	if textOf(item, "shared") != "" && kit.Button(c, upstreamT("Move into the library"), nil, false).Disabled(x.working).Clicked() {
		x.ask(upstreamT("Move into the library"), "/api/library/skills/hand-over", map[string]string{"name": name}, x.acceptLibrary)
	}
}
func (x *externalPanel) projectButtons(c *ui.Context, item map[string]any) {
	dir := textOf(item, "dir")
	copy := boolOf(item, "copy")
	if ui.Checkbox(c, &copy, upstreamT("Copies")).Disabled(x.working).Changed() {
		x.act("projects/copy", map[string]any{"dir": dir, "copy": copy})
	}
	if kit.Button(c, upstreamT("Choose skills and servers"), nil, false).Disabled(x.working).Clicked() {
		x.begin("project-items", item)
	}
	if kit.Button(c, upstreamT("Open folder"), icons.FolderOpen, false).Disabled(x.working).Clicked() {
		x.request("POST", "/api/library/reveal", map[string]string{"path": dir}, func(json.RawMessage) {})
	}
}
func (x *externalPanel) foundView(c *ui.Context, kind string) {
	key := "foundServers"
	if kind == "skills" {
		key = "foundSkills"
	}
	found := rowsOf(x.library, key)
	if len(found) > 0 {
		kit.Strong(c, externalT("found"))
	}
	for _, row := range found {
		item := row
		if kind == "servers" {
			item = objectOf(row, "server")
		}
		name := textOf(item, "name")
		kit.Section(c.Key("found"+name), nil, name, textOf(item, "description"), func() {
			externalDetails(c, row)
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if kit.Button(c, upstreamT("Bring in"), nil, false).Disabled(x.working).Clicked() {
					x.act(kind+"/import", map[string]string{"name": name})
				}
				if kit.Button(c, upstreamT("Remove"), icons.Trash2, false).Disabled(x.working).Clicked() {
					x.ask(upstreamT("Remove"), "/api/library/"+kind+"/remove-found", map[string]string{"name": name}, x.acceptLibrary)
				}
			})
		})
	}
	if kind == "skills" {
		for _, item := range rowsOf(x.library, "newSkills") {
			name := textOf(item, "name")
			kit.Strong(c, name)
			externalDetails(c, item)
			if kit.Button(c, upstreamT("Add"), nil, false).Disabled(x.working).Clicked() {
				x.act("skills/add-new", map[string]any{"names": []string{name}})
			}
			if kit.Button(c, upstreamT("Ignore"), nil, false).Disabled(x.working).Clicked() {
				x.act("skills/ignore-new", map[string]any{"names": []string{name}})
			}
		}
	}
}
func (x *externalPanel) libraryOptions(c *ui.Context, kind string) {
	if kind == "servers" {
		hub := boolOf(x.library, "mcpHub")
		if ui.Checkbox(c, &hub, upstreamT("MCP router")).Disabled(x.working).Changed() {
			x.act("servers/hub", map[string]any{"on": hub})
		}
		all := boolOf(x.library, "hubAllTools")
		if ui.Checkbox(c, &all, upstreamT("Give every tool")).Disabled(x.working).Changed() {
			x.act("servers/hub-tools", map[string]any{"on": all})
		}
		if kit.Button(c, upstreamT("Router description"), nil, false).Disabled(x.working).Clicked() {
			x.begin("hub-note", map[string]any{"note": textOf(x.library, "hubNote")})
		}
		if kit.Button(c, upstreamT("Add to another app"), nil, false).Disabled(x.working).Clicked() {
			x.begin("hub-open", nil)
		}
		return
	}
	router := boolOf(x.library, "skillRouter")
	if ui.Checkbox(c, &router, upstreamT("Skill router")).Disabled(x.working).Changed() {
		x.act("skills/router", map[string]any{"on": router})
	}
	shared := boolOf(x.library, "sharedSkills")
	if ui.Checkbox(c, &shared, upstreamT("Shared skills folder")).Disabled(x.working).Changed() {
		how := "agents"
		if shared {
			how = "shared"
		}
		x.ask(upstreamT("Shared skills folder"), "/api/library/skills/where", map[string]string{"how": how}, x.acceptLibrary)
	}
	how := "link"
	if boolOf(x.library, "copySkills") {
		how = "copy"
	}
	if nativeSelect(c, upstreamT("Give skills as"), &how, options("link", upstreamT("Links"), "copy", upstreamT("Copies")), x.working) {
		x.act("skills/how", map[string]string{"how": how})
	}
	for _, a := range rowsOf(x.library, "agents") {
		if textOf(a, "skills") == "" {
			continue
		}
		id := textOf(a, "id")
		how := ""
		if boolOf(a, "howOwn") {
			how = textOf(a, "how")
		}
		if nativeSelect(c, textOf(a, "name"), &how, options("", upstreamT("Library's way"), "link", upstreamT("Links"), "copy", upstreamT("Copies")), x.working || boolOf(a, "mustCopy")) {
			x.act("skills/how", map[string]string{"agent": id, "how": how})
		}
	}
	for _, group := range rowsOf(x.library, "skillGroups") {
		kit.Help(c, textOf(group, "name")+" · "+strings.Join(stringsOf(group["skills"]), ", "))
		if kit.Button(c, upstreamT("Edit group"), nil, false).Disabled(x.working).Clicked() {
			x.begin("skill-group", group)
		}
	}
}

func (x *externalPanel) field(c *ui.Context, e *libraryEditor, key, label string, multi bool) {
	value := e.values[key]
	if multi {
		kit.Help(c, upstreamT(label))
		ui.TextArea(c, &value).FillWidth().MinHeight(100).Disabled(x.working).Label(upstreamT(label))
	} else if key == "clientSecret" {
		ui.Field(c, upstreamT(label), func() { ui.TextInput(c, &value).FillWidth().Password().Disabled(x.working).Label(upstreamT(label)) })
	} else {
		nativeText(c, upstreamT(label), &value, x.working)
	}
	e.values[key] = value
}
func (x *externalPanel) choice(c *ui.Context, e *libraryEditor, key, label string, choices []kit.SelectOption) {
	v := e.values[key]
	nativeSelect(c, upstreamT(label), &v, choices, x.working)
	e.values[key] = v
}
func (x *externalPanel) toggle(c *ui.Context, e *libraryEditor, key, label string) {
	v := e.values[key] == "true"
	ui.Checkbox(c, &v, upstreamT(label)).Disabled(x.working)
	e.values[key] = strconv.FormatBool(v)
}
func (x *externalPanel) folder(c *ui.Context, e *libraryEditor, key string) {
	value := e.values[key]
	target := ui.TextInput(c, &value).FillWidth().Label(upstreamT("Folder")).Disabled(x.working)
	e.values[key] = value
	widgets.DropArea(c, x.p.env, target, widgets.DropFolders, false, x.working, func(paths []string, err error) {
		if err != nil {
			x.err = err.Error()
		} else if len(paths) > 0 {
			e.values[key] = paths[0]
		}
	})
	if kit.Button(c, upstreamT("Choose…"), icons.FolderOpen, false).Disabled(x.working).Clicked() {
		x.working = true
		core.Go(x.p.env, func() (string, error) { return x.p.env.Dialogs.PickFolder(upstreamT("Choose a project")) }, func(path string, err error) {
			x.working = false
			if err != nil {
				x.err = err.Error()
			} else if path != "" {
				e.values[key] = path
			}
		})
	}
}

func (x *externalPanel) editorView(c *ui.Context) {
	e := x.edit
	ui.Row(c).Gap(8).Wrap().Children(func() {
		kit.Strong(c, upstreamT("Edit")).Grow(1)
		if kit.PrimaryButton(c, upstreamT("Save"), icons.Save, x.working).Disabled(x.working).Clicked() {
			x.saveEditor()
		}
		if kit.Button(c, i18n.T("common.cancel"), nil, false).Disabled(x.working).Clicked() {
			if x.p.env.RequestChange != nil {
				x.p.env.RequestChange(x.discard)
			} else {
				x.discard()
			}
		}
	})
	if strings.HasPrefix(e.kind, "assign-") {
		x.agentsView(c, e.agents)
		return
	}
	if strings.HasPrefix(e.kind, "actions-") {
		x.actionFields(c, e, strings.TrimPrefix(e.kind, "actions-"))
		return
	}
	switch e.kind {
	case "servers-import":
		ui.TextArea(c, &e.serverJSON).FillWidth().MinHeight(260).Font(kit.MonoFont).Label("JSON").Disabled(x.working)
		x.agentsView(c, e.agents)
		if x.result != nil {
			externalDetails(c, x.result)
		}
	case "servers":
		if !x.serverJSONView(c, e) {
			x.field(c, e, "name", "Name", false)
			x.choice(c, e, "transport", "Transport", options("stdio", "stdio", "http", "HTTP", "sse", "SSE"))
			// Match upstream's transport form: command fields belong to stdio;
			// remote addresses and sign-in settings belong to HTTP/SSE.
			if e.values["transport"] == "stdio" {
				x.field(c, e, "command", "Command", false)
				x.field(c, e, "args", "Arguments (one per line)", true)
				x.field(c, e, "env", "Environment (name=value)", true)
			} else {
				x.field(c, e, "url", "URL", false)
				x.field(c, e, "headers", "Headers (name=value)", true)
			}
			x.field(c, e, "startupTimeout", "Startup timeout (seconds)", false)
			oauth := objectOf(e.original, "oauth")
			for _, k := range []string{"clientId", "clientSecret", "callbackPort"} {
				if _, ok := e.values[k]; !ok {
					e.values[k] = fmt.Sprint(oauth[k])
					if oauth[k] == nil {
						e.values[k] = ""
					}
				}
				if e.values["transport"] == "http" {
					x.field(c, e, k, map[string]string{"clientId": "Client ID", "clientSecret": "Client secret", "callbackPort": "Callback port"}[k], false)
				}
			}
		}
		x.field(c, e, "disabledTools", "Disabled tools (one per line)", true)
		x.field(c, e, "note", "Description", true)
		sync := e.values["local"] != "true"
		if ui.Checkbox(c, &sync, upstreamT("Sync to your other computers")).Disabled(x.working || e.original["bridge"] != nil).Changed() {
			e.values["local"] = strconv.FormatBool(!sync)
		}
		if boolOf(x.library, "mcpHub") || e.values["hub"] == "direct" {
			router := e.values["hub"] != "direct"
			if ui.Checkbox(c, &router, upstreamT("Through the MCP router")).Disabled(x.working).Changed() {
				e.values["hub"] = ""
				if !router {
					e.values["hub"] = "direct"
				}
			}
		}
		x.agentsView(c, e.agents)
	case "hooks":
		for _, pair := range [][2]string{{"name", "Name"}, {"description", "Description"}, {"command", "Command"}, {"matcher", "Matcher"}, {"timeout", "Timeout (seconds)"}} {
			x.field(c, e, pair[0], pair[1], pair[0] == "command")
		}
		choices := options("turn-end", upstreamT("When a turn ends"), "after-tool", upstreamT("After a tool runs"), "before-tool", upstreamT("Before a tool runs"), "prompt", upstreamT("When a prompt is sent"), "session-start", upstreamT("When a session starts"))
		for _, a := range rowsOf(x.library, "hookAgents") {
			for _, event := range stringsOf(a["events"]) {
				choices = append(choices, kit.SelectOption{Value: textOf(a, "id") + ":" + event, Label: textOf(a, "id") + " · " + event})
			}
		}
		x.choice(c, e, "event", "Event", choices)
		if e.values["event"] == "before-tool" || e.values["event"] == "after-tool" {
			x.choice(c, e, "matcher", "Tools", options("", upstreamT("Any tool"), "shell", upstreamT("Shell commands"), "edit", upstreamT("File edits")))
		}
		if e.values["event"] == "session-start" {
			x.choice(c, e, "matcher", "When", options("", upstreamT("Any start"), "startup", upstreamT("A fresh session"), "resume", upstreamT("A resumed session"), "clear", upstreamT("After /clear"), "compact", upstreamT("After the context is compacted"), "fork", upstreamT("A forked session"), "startup|resume|clear|compact", upstreamT("Any start but a fork")))
		}
		x.agentsView(c, e.agents)
		for _, a := range rowsOf(x.library, "hookAgents") {
			externalDetails(c, a)
		}
	case "subagents":
		for _, pair := range [][2]string{{"name", "Name"}, {"description", "Description"}, {"prompt", "Instructions"}} {
			x.field(c, e, pair[0], pair[1], pair[0] == "prompt")
		}
		choices := options("", upstreamT("Default"))
		for _, m := range rowsOf(x.library, "subagentModels") {
			choices = append(choices, kit.SelectOption{Value: textOf(m, "id"), Label: textOf(m, "name")})
		}
		x.choice(c, e, "model", "Model", choices)
		x.namedChecks(c, e, "skills", rowsOf(x.library, "skills"))
		x.namedChecks(c, e, "mcp", rowsOf(x.library, "servers"))
		x.agentsView(c, e.agents)
		for _, a := range rowsOf(x.library, "subagentAgents") {
			externalDetails(c, a)
		}
	case "instructions":
		x.field(c, e, "text", "Instructions", true)
	case "instruction-create", "instruction-rename":
		x.field(c, e, "name", "Name", false)
		x.field(c, e, "file", "Linked file (optional)", false)
	case "instruction-extra":
		x.field(c, e, "text", "Extra instructions", true)
	case "projects":
		x.folder(c, e, "dir")
	case "skills", "skills-install":
		x.field(c, e, "source", "Repository URL or folder", false)
		x.folder(c, e, "source")
		x.field(c, e, "paths", "Skills to install (one path per line)", true)
		x.agentsView(c, e.agents)
		if kit.Button(c, upstreamT("Find skills"), nil, false).Disabled(x.working).Clicked() {
			x.request("POST", "/api/library/skills/probe", map[string]string{"source": e.values["source"]}, x.receiveResult)
		}
		if x.result != nil {
			externalDetails(c, x.result)
		}
	case "skill-note", "hub-note":
		x.field(c, e, "note", "Note", true)
	case "skill-group":
		x.field(c, e, "name", "Group name", false)
		x.namedChecks(c, e, "skills", rowsOf(x.library, "skills"))
	case "server-tools":
		x.field(c, e, "disabledTools", "Disabled tools (one per line)", true)
	case "hub-open":
		x.choice(c, e, "app", "App", options("cursor", "Cursor", "vscode", "VS Code"))
	case "mcp-signin":
		externalDetails(c, e.original)
		id := e.values["id"]
		if kit.Button(c, upstreamT("Check sign-in"), nil, false).Disabled(x.working).Clicked() {
			x.request("GET", "/api/library/mcp-signin/"+url.PathEscape(id), nil, func(raw json.RawMessage) {
				var state map[string]any
				if json.Unmarshal(raw, &state) == nil {
					e.original = state
				}
			})
		}
		if kit.Button(c, upstreamT("Cancel sign-in"), nil, false).Disabled(x.working).Clicked() {
			x.request("POST", "/api/library/mcp-signin/"+url.PathEscape(id)+"/cancel", nil, func(json.RawMessage) { x.edit = nil })
		}
	case "project-items":
		x.choice(c, e, "what", "Type", options("skill", upstreamT("Skills"), "server", upstreamT("MCP servers")))
		rows := rowsOf(x.library, "skills")
		if e.values["what"] == "server" {
			rows = rowsOf(x.library, "servers")
		}
		choices := options("", upstreamT("Choose"))
		for _, item := range rows {
			choices = append(choices, kit.SelectOption{Value: textOf(item, "name"), Label: textOf(item, "name")})
		}
		x.choice(c, e, "name", "Item", choices)
		x.agentsView(c, e.agents)
	case "market-server", "market-skill":
		kit.Help(c, textOf(e.original, "description"))
		for _, field := range rowsOf(e.original, "inputs") {
			key := textOf(field, "key")
			x.field(c, e, key, textOf(field, "label"), false)
			kit.Help(c, textOf(field, "description"))
		}
		x.agentsView(c, e.agents)
	default:
		externalDetails(c, e.original)
	}
}
func (x *externalPanel) namedChecks(c *ui.Context, e *libraryEditor, key string, items []map[string]any) {
	selected := selectedStrings(listLines(e.values[key]))
	kit.Strong(c, upstreamT(key))
	for _, item := range items {
		name := textOf(item, "name")
		on := selected[name]
		if ui.Checkbox(c.Key(key+name), &on, name).Disabled(x.working).Changed() {
			selected[name] = on
			e.values[key] = strings.Join(picked(selected), "\n")
		}
	}
}
func (x *externalPanel) saveEditor() {
	e := x.edit
	if e == nil {
		return
	}
	v := e.values
	if strings.HasPrefix(e.kind, "assign-") {
		x.act(strings.TrimPrefix(e.kind, "assign-")+"/agents", map[string]any{"name": e.old, "agents": picked(e.agents)})
		return
	}
	if strings.HasPrefix(e.kind, "actions-") {
		x.saveAction(e)
		return
	}
	switch e.kind {
	case "servers-import":
		x.saveServerImport(e)
	case "servers", "hooks", "subagents":
		if e.kind == "servers" && e.serverView == 1 {
			if err := e.readServerJSON(); err != nil {
				e.serverError, x.err = err.Error(), err.Error()
				return
			}
		}
		item := map[string]any{}
		for k, value := range e.original {
			item[k] = value
		}
		fields := map[string][]string{"servers": {"name", "transport", "command", "url", "note", "hub"}, "hooks": {"name", "description", "event", "matcher", "command"}, "subagents": {"name", "description", "prompt", "model"}}
		for _, key := range fields[e.kind] {
			item[key] = v[key]
		}
		item["agents"] = picked(e.agents)
		switch e.kind {
		case "servers":
			item["local"] = v["local"] == "true"
			item["args"] = e.serverField("args")
			item["env"] = e.serverField("env")
			item["headers"] = e.serverField("headers")
			item["disabledTools"] = listLines(v["disabledTools"])
			n, err := strconv.Atoi("0" + v["startupTimeout"])
			if err != nil {
				x.err = err.Error()
				return
			}
			item["startupTimeout"] = n
			item["oauth"] = nil
			if v["transport"] == "http" && v["clientId"] != "" {
				port, err := strconv.Atoi("0" + v["callbackPort"])
				if err != nil {
					x.err = err.Error()
					return
				}
				item["oauth"] = map[string]any{"clientId": v["clientId"], "clientSecret": v["clientSecret"], "callbackPort": port}
			}
			for _, key := range []string{"clientId", "clientSecret", "callbackPort"} {
				delete(item, key)
			}
			if v["transport"] == "stdio" {
				delete(item, "url")
				delete(item, "headers")
			} else {
				delete(item, "command")
				delete(item, "args")
				delete(item, "env")
			}
		case "hooks":
			n, err := strconv.Atoi("0" + v["timeout"])
			if err != nil {
				x.err = err.Error()
				return
			}
			item["timeout"] = n
		case "subagents":
			item["skills"] = listLines(v["skills"])
			item["mcp"] = listLines(v["mcp"])
		}
		singular := map[string]string{"servers": "server", "hooks": "hook", "subagents": "subagent"}[e.kind]
		x.act(e.kind+"/save", map[string]any{"old": e.old, singular: item})
	case "instructions":
		x.act("instructions/save", map[string]any{"texts": map[string]string{v["id"]: v["text"]}})
	case "instruction-create", "instruction-rename":
		key := "create"
		if e.kind == "instruction-rename" {
			key = "rename"
		}
		x.act("instructions/save", map[string]any{key: map[string]string{"id": v["id"], "name": v["name"], "file": v["file"]}})
	case "instruction-extra":
		x.act("instructions/save", map[string]any{"extra": map[string]string{v["agent"]: v["text"]}})
	case "projects":
		x.act("projects/add", map[string]string{"dir": v["dir"]})
	case "skills", "skills-install":
		x.ask(upstreamT("Install skills"), "/api/library/skills/install", map[string]any{"source": v["source"], "paths": listLines(v["paths"]), "agents": picked(e.agents)}, x.acceptLibrary)
	case "skill-note":
		x.act("skills/note", map[string]string{"name": e.old, "note": v["note"]})
	case "hub-note":
		x.act("servers/hub-note", map[string]string{"note": v["note"]})
	case "hub-open":
		x.act("servers/hub-open", map[string]string{"app": v["app"]})
	case "skill-group":
		x.act("skills/group", map[string]any{"old": e.old, "name": v["name"], "names": listLines(v["skills"])})
	case "server-tools":
		x.act("servers/tools", map[string]any{"name": e.old, "names": listLines(v["disabledTools"])})
	case "project-items":
		what := v["what"]
		if what == "" {
			what = "skill"
		}
		x.act("projects/"+what, map[string]any{"dir": v["dir"], "name": v["name"], "agents": picked(e.agents)})
	case "market-server", "market-skill":
		body := map[string]any{"id": v["id"], "agents": picked(e.agents), "source": v["source"]}
		if e.kind == "market-server" {
			values := map[string]string{}
			for _, field := range rowsOf(e.original, "inputs") {
				key := textOf(field, "key")
				values[key] = v[key]
			}
			body["values"] = values
		} else {
			body["id"] = v["skillId"]
		}
		path := "/api/library/market/" + strings.TrimPrefix(e.kind, "market-")
		x.ask(upstreamT("Add to the library"), path, body, x.acceptLibrary)
	case "mcp-signin":
		x.edit = nil
	}
}

// Batch actions keep the upstream scope distinctions: all items, selected
// items, discovered items, and one conflicting client's own copy.
var libraryBatch = map[string][]string{
	"servers": {"agents-all", "agents-some", "remove-all", "config", "plugin-on"},
	"skills":  {"agents-all", "agents-some", "update-some", "remove-all", "remove-gone", "import-all", "remove-found-some", "add-new", "ignore-new", "group", "ungroup", "hand-over", "use-library", "keep-own"},
	"hooks":   {"agents-some", "remove-all"}, "subagents": {}, "projects": {"remove"},
}

func (x *externalPanel) actionFields(c *ui.Context, e *libraryEditor, kind string) {
	choices := []kit.SelectOption{}
	for _, action := range libraryBatch[kind] {
		choices = append(choices, kit.SelectOption{Value: action, Label: upstreamT(action)})
	}
	if e.values["action"] == "" && len(choices) > 0 {
		e.values["action"] = choices[0].Value
	}
	x.choice(c, e, "action", "Action", choices)
	if _, ok := e.values["names"]; !ok {
		e.values["names"] = strings.Join(picked(x.selected), "\n")
	}
	x.field(c, e, "names", "Names (one per line)", true)
	x.field(c, e, "name", "Name or group", false)
	x.field(c, e, "old", "Previous group name", false)
	choices = options("", upstreamT("Choose client"))
	for _, a := range rowsOf(x.library, "agents") {
		choices = append(choices, kit.SelectOption{Value: textOf(a, "id"), Label: textOf(a, "name")})
	}
	x.choice(c, e, "agent", "Client", choices)
	x.agentsView(c, e.agents)
	x.toggle(c, e, "on", "On")
	if kind == "projects" {
		x.folder(c, e, "dir")
		x.toggle(c, e, "keep", "Keep files")
	}
}
func (x *externalPanel) saveAction(e *libraryEditor) {
	kind := strings.TrimPrefix(e.kind, "actions-")
	action := e.values["action"]
	if !slices.Contains(libraryBatch[kind], action) {
		x.err = upstreamT("Choose an action")
		return
	}
	body := map[string]any{"names": listLines(e.values["names"]), "name": e.values["name"], "old": e.values["old"], "agent": e.values["agent"], "agents": picked(e.agents), "on": e.values["on"] == "true", "dir": e.values["dir"], "keep": e.values["keep"] == "true"}
	if action == "config" {
		x.request("POST", "/api/library/mcp/config", body, x.receiveResult)
		return
	}
	x.ask(upstreamT(action), "/api/library/"+kind+"/"+action, body, x.acceptLibrary)
}

func (x *externalPanel) readSkill(name, file string) {
	x.request("GET", "/api/library/skill?name="+url.QueryEscape(name)+"&file="+url.QueryEscape(file), nil, func(raw json.RawMessage) {
		var f librarySkillFile
		if err := json.Unmarshal(raw, &f); err != nil {
			x.err = err.Error()
			return
		}
		f.Name = name
		f.Was = f.Text
		x.skill = &f
	})
}
func (x *externalPanel) skillView(c *ui.Context) {
	f := x.skill
	ui.Row(c).Gap(8).Wrap().Children(func() {
		kit.Strong(c, f.Name).Grow(1)
		if kit.PrimaryButton(c, upstreamT("Save"), icons.Save, x.working).Disabled(x.working || f.Text == f.Was).Clicked() {
			x.request("POST", "/api/library/skills/save", map[string]string{"name": f.Name, "file": f.File, "text": f.Text, "was": f.Was}, func(raw json.RawMessage) {
				x.acceptLibrary(raw)
				if x.err == "" {
					f.Was = f.Text
				}
			})
		}
		if kit.Button(c, i18n.T("common.close"), nil, false).Disabled(x.working).Clicked() {
			if f.Text != f.Was && x.p.env.RequestChange != nil {
				x.p.env.RequestChange(func() { x.skill = nil })
			} else {
				x.skill = nil
			}
		}
	})
	choices := []kit.SelectOption{}
	for _, file := range f.Files {
		choices = append(choices, kit.SelectOption{Value: file, Label: file})
	}
	file := f.File
	if nativeSelect(c, externalT("files"), &file, choices, x.working || f.Text != f.Was) {
		x.readSkill(f.Name, file)
	}
	ui.TextArea(c, &f.Text).FillWidth().MinHeight(400).Label(f.File).Font(kit.MonoFont).Disabled(x.working)
}

func (x *externalPanel) marketView(c *ui.Context) {
	ui.TextInput(c, &x.marketQuery).FillWidth().Label(upstreamT("Search"))
	if kit.PrimaryButton(c, externalT("market"), icons.Search, x.working).Disabled(x.working).Clicked() {
		x.request("GET", "/api/library/market/"+x.marketKind+"?q="+url.QueryEscape(x.marketQuery), nil, func(raw json.RawMessage) {
			var state map[string]any
			if json.Unmarshal(raw, &state) != nil {
				x.err = nativeT("invalidResponse")
				return
			}
			x.market = rowsOf(state, "items")
			x.message = textOf(state, "error")
			if custom := objectOf(state, "custom"); custom != nil {
				custom["custom"] = true
				x.market = append(x.market, custom)
			}
		})
	}
	for _, item := range x.market {
		id := textOf(item, "id")
		title := textOf(item, "title")
		if title == "" {
			title = textOf(item, "name")
		}
		kit.Section(c.Key(id+title), nil, title, textOf(item, "description"), func() {
			for _, key := range []string{"publisher", "homepage", "runs", "unready", "have"} {
				if value := textOf(item, key); value != "" {
					kit.Help(c, upstreamT(key)+": "+value)
				}
			}
			if kit.Button(c, upstreamT("Add to the library"), icons.Plus, false).Disabled(x.working || textOf(item, "unready") != "").Clicked() {
				if boolOf(item, "custom") {
					// An address the market does not know is a new editable server,
					// with the same client defaults as the ordinary Add action.
					x.begin("servers", nil)
					for _, key := range []string{"name", "transport", "url", "command"} {
						x.edit.values[key] = textOf(item, key)
					}
					x.edit.values["args"] = strings.Join(stringsOf(item["args"]), "\n")
					return
				}
				kind := "market-server"
				if x.marketKind == "skills" {
					kind = "market-skill"
				}
				x.begin(kind, item)
			}
			if x.marketKind == "skills" && kit.Button(c, upstreamT("Description"), nil, false).Disabled(x.working).Clicked() {
				x.request("POST", "/api/library/market/about", map[string]any{"ids": []string{id}}, x.receiveResult)
			}
		})
	}
	if kit.Button(c, upstreamT("Set up the extension…"), nil, false).Disabled(x.working).Clicked() {
		x.request("GET", "/api/library/browser", nil, x.receiveResult)
	}
	if kit.Button(c, upstreamT("Write browser extension"), nil, false).Disabled(x.working).Clicked() {
		x.ask(upstreamT("Set up the extension…"), "/api/library/browser/setup", map[string]any{}, x.receiveResult)
	}
	if kit.Button(c, upstreamT("Stop browser extension"), nil, false).Disabled(x.working).Clicked() {
		x.request("POST", "/api/winone/external/browser/stop", map[string]any{}, x.receiveResult)
	}
}
func (x *externalPanel) rtkView(c *ui.Context) {
	accept := func(raw json.RawMessage) {
		if err := json.Unmarshal(raw, &x.rtk); err != nil {
			x.err = err.Error()
		}
	}
	if kit.PrimaryButton(c, externalT("readRTK"), icons.RefreshCw, x.working).Disabled(x.working).Clicked() {
		x.request("GET", "/api/library/rtk", nil, accept)
	}
	if x.rtk == nil {
		return
	}
	for _, key := range []string{"path", "version", "latest", "install", "upgrade", "manual", "noUpgrade", "waiting", "waitingHas", "note", "pathDir", "backup", "gainErr"} {
		if value := textOf(x.rtk, key); value != "" {
			kit.Help(c, upstreamT(key)+": "+value)
		}
	}
	externalDetails(c, x.rtk["gain"])
	externalDetails(c, x.rtk["days"])
	for _, action := range []string{"install", "upgrade", "path"} {
		if kit.Button(c, upstreamT(map[string]string{"install": "Install RTK", "upgrade": "Upgrade RTK", "path": "Put RTK on PATH"}[action]), nil, false).Disabled(x.working).Clicked() {
			x.ask(upstreamT("RTK"), "/api/library/rtk/"+action, map[string]any{}, accept)
		}
	}
	for _, a := range rowsOf(x.rtk, "agents") {
		id := textOf(a, "id")
		on := boolOf(a, "on")
		if ui.Checkbox(c.Key(id), &on, textOf(a, "name")).Disabled(x.working || boolOf(a, "noHook") || boolOf(a, "asleep") || textOf(a, "blocked") != "" && !on).Changed() {
			x.request("POST", "/api/library/rtk", map[string]any{"agent": id, "on": on}, accept)
		}
		kit.Help(c, textOf(a, "blocked"))
	}
	for _, d := range rowsOf(x.rtk, "wsl") {
		externalDetails(c, d)
		if kit.Button(c, upstreamT("Install RTK")+" · "+textOf(d, "name"), nil, false).Disabled(x.working).Clicked() {
			x.ask(upstreamT("Install RTK"), "/api/library/rtk/install", map[string]string{"distro": textOf(d, "name")}, accept)
		}
	}
}
