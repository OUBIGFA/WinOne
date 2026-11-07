package chat

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"winone/desktopmygo/chat/pluginhost"
	"winone/desktopmygo/chat/workspace"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// toolDialog is a file tool's snapshot dialog, as AgentActionRow's: the
// tool as its row last showed it, whether a Markdown document shows as
// source, and the image a read of a picture loads.
type toolDialog struct {
	key     string
	tool    model.Tool
	source  bool
	raw     bool
	image   ui.ImageSource
	imageOf string
	err     string
}

// toolRow is a tool step of a work process, as Cindy's AgentActionRow:
// its state, verb and argument; a command shows its full command line
// below when the row names it otherwise. A click opens the call's input
// and output, or, for a read, write or edit of one file, the dialog with
// what the tool read or changed.
// toolLabel is a tool's row text: its verb and what it acts on.
func toolLabel(tool model.Tool) string {
	d := model.DescribeTool(tool.Name, tool.Args)
	verb := tool.Name
	switch {
	case d.Kind == "file":
		verb = i18n.T("chat.toolVerb_" + d.Action)
	case verb == "":
		verb = i18n.T("chat.tool")
	}
	if param := d.DisplayParam(tool.Args); param != "" {
		return verb + " " + param
	}
	return verb
}

// teammateWorkLabel names the latest tool of a teammate's work, running or not.
func teammateWorkLabel(entries []model.Entry) string {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Kind == "tool" {
			return toolLabel(entries[i].Tool)
		}
	}
	return ""
}

func (p *Chat) toolRow(c *ui.Context, e model.Entry) {
	t := c.Theme()
	tool := e.Tool
	d := model.DescribeTool(tool.Name, tool.Args)
	file := d.IsFileTool()
	verb := tool.Name
	switch {
	case d.Kind == "file":
		verb = i18n.T("chat.toolVerb_" + d.Action)
	case verb == "":
		verb = i18n.T("chat.tool")
	}
	param := d.DisplayParam(tool.Args)
	command := d.Command
	if d.Kind != "command" {
		command, _ = tool.Args["command"].(string)
	}
	key := "tool:" + e.Key
	open := p.ui.expanded[key] && !file
	if dialog := p.ui.items.tool; dialog != nil && dialog.key == key {
		dialog.tool = tool
	}
	state, stateIcon := i18n.T("chat.toolDone"), icons.Check
	if tool.IsError {
		state, stateIcon = i18n.T("chat.failed"), icons.TriangleAlert
	}
	label := toolLabel(tool)
	ui.Column(c.Key(key)).FillWidth().MinWidth(0).Children(func() {
		head := ui.ButtonBase(c).FillWidth().Gap(6).Justify(ui.Start).AlignItems(ui.Center).Padding(3, 8).Radius(8).Label(label)
		if !file {
			head.Expanded(open)
		}
		head.Children(func() {
			ui.Row(c).Size(16, 18).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				if tool.Running {
					ui.Spinner(c).Size(13, 13).Label(i18n.T("chat.working"))
				} else {
					ui.Icon(c, stateIcon).FontSize(13).TextColor(t.TextMuted).Label(state)
				}
			})
			ui.Text(c, verb).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted).SingleLine().Shrink(0)
			if file {
				ui.Icon(c, icons.FileText).FontSize(14).TextColor(t.TextMuted).Shrink(0)
			}
			tip := param
			if file {
				tip = d.FilePath
			}
			ui.Text(c, param).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1).Tooltip(tip)
			ui.Spacer(c)
			chevron := icons.ChevronRight
			if open {
				chevron = icons.ChevronDown
			}
			ui.Icon(c, chevron).FontSize(13).TextColor(t.TextMuted).Shrink(0)
		})
		if head.Clicked() {
			if file {
				p.ui.items.tool = &toolDialog{key: key, tool: tool}
			} else {
				p.ui.expanded[key] = !open
			}
		}
		if id := pluginCardID(tool); id != "" {
			pluginhost.For(p.env).Card(c, id)
		}
		if command != "" && command != param && !open {
			kit.Mono(c, command).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MinWidth(0).
				Padding(0, 8, 3, 30).Tooltip(command)
		}
		if open {
			ui.Column(c).FillWidth().Margin(4, 0).Padding(8, 10).Radius(6).Border(1, kit.Colors(c).Hairline).Background(kit.Colors(c).Panel).
				Children(func() {
					p.toolPayload(c, tool)
					p.toolImageViews(c, toolImagePaths(tool))
				})
		}
	})
}

var pluginCardPattern = regexp.MustCompile(`\\?"xdt_card_id\\?"\s*:\s*\\?"([^"\\]+)`)

// pluginCardID is the card a plugin's tool call supplied, from the token Cindy's
// switchboard puts in its result. The model never sees more than that id.
func pluginCardID(tool model.Tool) string {
	if tool.Name != "ghost_call" {
		return ""
	}
	if match := pluginCardPattern.FindStringSubmatch(resultText(tool.Result)); match != nil {
		return match[1]
	}
	return ""
}

// toolInput is the call's input as the details show it: a command alone
// when that is all it has, else its arguments as JSON.
func toolInput(tool model.Tool) string {
	if command, ok := tool.Args["command"].(string); ok && command != "" {
		only := true
		for key := range tool.Args {
			only = only && (key == "command" || key == "cwd" || key == "description")
		}
		if only {
			return command
		}
	}
	data, _ := json.MarshalIndent(tool.Args, "", "  ")
	return string(data)
}

// toolPayload is a call's input and output, each with Copy.
func (p *Chat) toolPayload(c *ui.Context, tool model.Tool) {
	t := c.Theme()
	section := func(label, text string, failed bool) {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted).Grow(1)
			if kit.IconButton(c, icons.Copy, i18n.T("chat.copy")).Size(24, 24).Clicked() {
				p.copy(text)
			}
		})
		out := kit.Mono(c, truncate(text, 20000)).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted).Selectable().MinWidth(0)
		if failed {
			out.TextColor(t.Text)
		}
	}
	input := toolInput(tool)
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		section(i18n.T("chat.toolInput"), input, false)
		if tool.Result != nil {
			ui.Box(c).FillWidth().Height(1).Margin(4, 0).Background(kit.Colors(c).Hairline)
			section(i18n.T("chat.toolOutput"), resultText(tool.Result), tool.IsError)
		}
	})
}

// toolImagePaths accepts only the real worker's persisted image metadata. A
// data-only compatibility part never becomes a fabricated file or URL here.
func toolImagePaths(tool model.Tool) []string {
	var paths []string
	seen := map[string]bool{}
	for _, part := range tool.ResultParts {
		if part.Type == "image" && part.Path != "" && !seen[part.Path] {
			seen[part.Path] = true
			paths = append(paths, part.Path)
		}
	}
	return paths
}

// Tool results share the native document/image viewer, including zoom and
// rotation. Its preview authority is still this conversation's Host fileRead
// boundary; neither tool arguments nor display metadata grant filesystem access.
func (p *Chat) toolImageViews(c *ui.Context, paths []string) {
	for _, path := range paths {
		if p.toolMedia == nil {
			p.toolMedia = map[string]*workspace.DocumentPane{}
		}
		pane := p.toolMedia[path]
		if pane == nil {
			pane = workspace.NewDocumentPane(p.env, workspace.DocumentOptions{Prefix: p.method(""), SessionID: p.activeID, Path: path})
			p.toolMedia[path] = pane
		}
		p.mediaShown["tool:"+path] = true
		ui.Column(c.Key("tool-image:" + path)).FillWidth().MinWidth(0).Height(320).Children(func() { pane.View(c) })
	}
}

func (p *Chat) closeToolMedia() {
	for _, pane := range p.toolMedia {
		pane.Close()
	}
	p.toolMedia = nil
}

// operationRow is an operation of a turn, as WorkProcess's Operation:
// compaction, review or approval, its attempt, state and time.
func (p *Chat) operationRow(c *ui.Context, op model.Operation) {
	t := c.Theme()
	icon := icons.Layers
	switch op.Kind {
	case "retry":
		icon = icons.RotateCcw
	case "review", "approval":
		icon = icons.ShieldCheck
	}
	label := i18n.T("chat.processWorking")
	if key, ok := operationLabels[op.Kind]; ok {
		label = i18n.T(key)
	}
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Start).Padding(2, 8).Children(func() {
		ui.Row(c).Size(16, 20).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			if op.Status == "running" {
				ui.Spinner(c).Size(13, 13)
			} else {
				ui.Icon(c, icon).FontSize(14).TextColor(t.TextMuted)
			}
		})
		ui.Row(c).Grow(1).MinWidth(0).Wrap().Gap(8).Children(func() {
			text := label
			if op.Attempt != nil {
				text += " · " + number(*op.Attempt)
				if op.MaxAttempts != nil {
					text += " / " + number(*op.MaxAttempts)
				}
			}
			ui.Text(c, text).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted)
			status := ui.Text(c, i18n.T("chat.operationStatus_"+op.Status)).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted)
			if op.Status == "failed" || op.Status == "denied" {
				status.TextColor(t.Text).FontWeight(600)
			}
		})
		if op.DurationMs != nil {
			kit.Mono(c, duration(op.DurationMs)).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted).Shrink(0)
		}
	})
}

// markdownFile reports a Markdown document, which the dialog renders.
func markdownFile(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range []string{".md", ".markdown", ".mdown", ".mkd"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func imageFile(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// fenced is text as a fenced code block of language, its fence longer
// than any run of backticks inside.
func fenced(language, text string) string {
	fence, run := "```", 0
	for _, r := range text {
		if r == '`' {
			run++
			if run >= len(fence) {
				fence += "`"
			}
		} else {
			run = 0
		}
	}
	return fence + language + "\n" + text + "\n" + fence
}

// toolDialogView is the open file tool's dialog, as ChatToolFilePreview:
// the snapshot the tool saw, never today's file, which a separate action
// opens in the side panel; the raw call below, open when there is no
// snapshot.
func (p *Chat) toolDialogView(c *ui.Context) {
	dialog := p.ui.items.tool
	if dialog == nil {
		return
	}
	t := c.Theme()
	tool := dialog.tool
	d := model.DescribeTool(tool.Name, tool.Args)
	name := strings.ToLower(tool.Name)
	path := model.ResolveToolFilePath(d.FilePath, p.directory())
	snapshot, hasSnapshot := model.ToolFileSnapshot(tool.Name, tool.Args, tool.Result)
	images := toolImagePaths(tool)
	legacyImage := len(images) == 0 && name == "read" && imageFile(d.FilePath) && !tool.IsError && !tool.Running && tool.Result != nil
	image := len(images) > 0 || legacyImage
	if legacyImage && dialog.imageOf != path {
		dialog.imageOf = path
		p.loadToolImage(dialog, path)
	}
	open := true
	verb := i18n.T("chat.toolVerb_" + d.Action)
	kit.Dialog(c, &open, kit.DialogOptions{Title: verb + " · " + d.FilePath, Message: i18n.T("chat.toolDetailHint"),
		Cancel: i18n.T("common.close"), Width: 760, Error: dialog.err, Body: func() {
			ui.Row(c).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
				label := i18n.T("chat.toolOutput")
				if hasSnapshot {
					label = i18n.T("chat.toolSnapshot_" + snapshot.Source)
				}
				switch {
				case tool.IsError:
					label += " · " + i18n.T("chat.failed")
				case tool.Running:
					label += " · " + i18n.T("chat.working")
				}
				if name == "read" && (tool.Args["offset"] != nil || tool.Args["limit"] != nil) {
					label += " · " + i18n.T("chat.toolReadExcerpt")
				}
				ui.Text(c, label).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted).Grow(1).MinWidth(0)
				if hasSnapshot && snapshot.Kind == "document" && markdownFile(d.FilePath) {
					label, icon := i18n.T("chat.fileSource"), icons.CodeXml
					if dialog.source {
						label, icon = i18n.T("chat.fileRendered"), icons.Eye
					}
					if kit.GhostButton(c, label, icon, false).Clicked() {
						dialog.source = !dialog.source
					}
				}
				if hasSnapshot && kit.IconButton(c, icons.Copy, i18n.T("chat.copy")).Clicked() {
					p.copy(snapshot.Text)
				}
				if kit.GhostButton(c, i18n.T("chat.openCurrentFile"), icons.FileText, false).Clicked() {
					p.openFile(path, 0)
					p.ui.items.tool = nil
				}
			})
			switch {
			case len(images) > 0:
				p.toolImageViews(c, images)
			case legacyImage:
				if dialog.image != nil {
					ui.Image(c, dialog.image).MaxWidthPercent(100).MaxHeight(480).Fit(ui.Contain).Label(d.FileName)
				} else if dialog.err == "" {
					kit.Loader(c)
				}
			case hasSnapshot && snapshot.Kind == "diff":
				markdown.View(c, fenced("diff", snapshot.Text), p.answerOptions(dialog.key)).FillWidth()
			case hasSnapshot && markdownFile(d.FilePath) && !dialog.source && !tool.IsError:
				markdown.View(c, snapshot.Text, p.answerOptions(dialog.key)).FillWidth()
			case hasSnapshot:
				kit.Mono(c, truncate(snapshot.Text, 200000)).Selectable().MinWidth(0)
			}
			raw := dialog.raw || !hasSnapshot && !image
			b := ui.ButtonBase(c).AlignSelf(ui.Start).Gap(4).AlignItems(ui.Center).Padding(2, 4).Radius(t.Radius).Label(i18n.T("chat.rawToolData")).Expanded(raw)
			b.Children(func() {
				chevron := icons.ChevronRight
				if raw {
					chevron = icons.ChevronDown
				}
				ui.Icon(c, chevron).FontSize(14).TextColor(t.TextMuted)
				ui.Text(c, i18n.T("chat.rawToolData")).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted)
			})
			if b.Clicked() {
				dialog.raw = !raw
			}
			if raw {
				p.toolPayload(c, tool)
			}
		}})
	if !open && p.ui.items.tool == dialog {
		p.ui.items.tool = nil
	}
}

// loadToolImage shows the picture a read looked at, through the Host's
// preview address, as ChatFileImage.
func (p *Chat) loadToolImage(dialog *toolDialog, path string) {
	chatCall(p, "file.preview", map[string]any{"id": p.activeID, "path": path}, func(r struct {
		URL string `json:"url"`
	}, err error) {
		if err != nil {
			dialog.err = err.Error()
			return
		}
		core.Go(p.env, func() (ui.ImageSource, error) {
			response, err := http.Get(r.URL)
			if err != nil {
				return nil, err
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				return nil, core.Errorf(response.Status)
			}
			data, err := io.ReadAll(io.LimitReader(response.Body, 100<<20))
			if err != nil {
				return nil, err
			}
			return ui.DecodeImage(data)
		}, func(image ui.ImageSource, err error) {
			if err != nil {
				dialog.err = err.Error()
				return
			}
			dialog.image = image
		})
	})
}
