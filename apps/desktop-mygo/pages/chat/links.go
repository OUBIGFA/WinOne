package chat

import (
	"strings"
	"winone/desktopmygo/chat/mathview"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// answerRefs are the local files a finished answer names, by its links
// and inline paths, read once per text.
type answerRefs struct {
	text string
	refs []answerRef
}

// answerRef is one of them: what the answer wrote and where it leads.
type answerRef struct {
	label  string
	target model.LinkTarget
	// link marks a Markdown link, which leads there whether or not the
	// file exists; inline code becomes a reference only once it does.
	link bool
}

// conversationFiles are the attachments of the open conversation, which
// a bare file name in an answer may cite.
func (p *Chat) attachments() []model.Attachment {
	var files []model.Attachment
	for _, m := range p.view.Messages {
		files = append(files, m.Attachments...)
	}
	return files
}

func (p *Chat) directory() string {
	if conversation := p.conversation(); conversation != nil {
		return conversation.Directory
	}
	return ""
}

// refsOf reads an answer's links and inline code for local paths, as
// Cindy's MarkdownRenderer routes links and turns unambiguous inline
// paths into file chips. Fenced code stays code.
func (p *Chat) refsOf(key, text string) []answerRef {
	s := &p.ui.items
	if cached, ok := s.refs[key]; ok && cached.text == text {
		return cached.refs
	}
	if s.refs == nil {
		s.refs = map[string]answerRefs{}
	}
	cwd, files := p.directory(), p.attachments()
	seen := map[string]bool{}
	var refs []answerRef
	add := func(ref answerRef) {
		if !seen[strings.ToLower(ref.target.Path)] {
			seen[strings.ToLower(ref.target.Path)] = true
			refs = append(refs, ref)
		}
	}
	doc := parser.Parse(text)
	for i := 0; i < doc.Len(); i++ {
		switch doc.Block(i).Kind {
		case parser.Paragraph, parser.Heading, parser.TableCell:
		default:
			continue
		}
		inlines := doc.Inlines(i)
		for j := 0; j < len(inlines); j++ {
			in := inlines[j]
			switch in.Kind {
			case parser.Link:
				if target := model.TargetOf(in.Dest, cwd, files); target.Local() {
					var label []string
					for k := j + 1; k < in.Next && k < len(inlines); k++ {
						if inlines[k].Kind == parser.Text || inlines[k].Kind == parser.Code {
							label = append(label, inlines[k].Text)
						}
					}
					add(answerRef{label: first(strings.Join(label, ""), in.Dest), target: target, link: true})
				}
				j = in.Next - 1
			case parser.Code:
				if target, ok := model.InlinePathCandidate(in.Text, cwd, files); ok {
					add(answerRef{label: in.Text, target: target})
				}
			}
		}
	}
	s.refs[key] = answerRefs{text: text, refs: refs}
	return refs
}

// answerFiles are the files a finished answer names, as chips under it:
// a click opens one in the side panel (Shift or Ctrl+click with the
// system), the context menu has the file's actions. Inline paths show
// only once the Host finds them, as Cindy's inline file chips.
func (p *Chat) answerFiles(c *ui.Context, key, text string) {
	type chip struct {
		ref  answerRef
		file fileStat
	}
	var chips []chip
	for _, ref := range p.refsOf(key, text) {
		check := p.checkFile(ref.target.Path)
		if check != nil && check.file != nil {
			chips = append(chips, chip{ref, *check.file})
		}
	}
	surface := "answer:" + key
	message := p.ui.items.errors[surface]
	if len(chips) == 0 && message == "" {
		return
	}
	s := &p.ui.items
	if s.refsOpen == nil {
		s.refsOpen = map[string]bool{}
	}
	open := s.refsOpen[key]
	ui.Column(c.Key("refs:" + key)).FillWidth().Gap(6).Children(func() {
		ui.Row(c).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
			shown := chips
			if !open && len(chips) > 6 {
				shown = chips[:6]
			}
			for _, ch := range shown {
				ref, file := ch.ref, ch.file
				label := ref.label
				if ref.target.Line > 0 && !strings.Contains(label, ":") {
					label += ":" + kit.Itoa(ref.target.Line)
				}
				chip := fileChip(c, "ref:"+file.Path, label, file.Path, file.Directory, func(m *ui.Menu) { p.fileMenu(m, ref.target, file, surface) }, func() { p.openTarget(ref.target, file, surface) })
				if chip.Clicked() {
					if chip.ClickModifiers()&(ui.Shift|ui.Ctrl) != 0 && !file.Directory {
						p.openDefault(file.Path, surface)
					} else {
						p.openTarget(ref.target, file, surface)
					}
				}
			}
			if len(chips) > 6 {
				moreToggle(c, &open, len(chips)-6)
				s.refsOpen[key] = open
			}
		})
		if message != "" {
			kit.InlineError(c, message)
		}
	})
}

// openTarget opens a file in the side panel at its line, or a folder in
// its file library.
func (p *Chat) openTarget(target model.LinkTarget, file fileStat, surface string) {
	p.setItemError(surface, "")
	if file.Directory {
		p.openPanel(panelRequest{Tab: "files", Directory: file.Path})
		return
	}
	p.openFile(file.Path, target.Line)
}

func isHTML(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm") || strings.HasSuffix(lower, ".xhtml")
}

// openDefault opens a file with the system: a web page through its
// preview address in the browser, since the default app for HTML may be
// an editor.
func (p *Chat) openDefault(path, surface string) {
	if isHTML(path) {
		p.openPreview(path, surface, true)
		return
	}
	p.fileAction("file.open", path, surface)
}

// fileMenu is a file reference's context menu, as Cindy's
// useFileChipContextMenu: a page opens in the side panel's browser, the
// system's, or as source; any other file in the side panel or with the
// system; then reveal, copy the file, copy its path and its line.
func (p *Chat) fileMenu(m *ui.Menu, target model.LinkTarget, file fileStat, surface string) {
	path := file.Path
	html := isHTML(path) && !file.Directory
	if html {
		if m.Item(i18n.T("chat.fileMenuSidebarBrowser")).Chosen() {
			p.openPreview(path, surface, false)
		}
		if m.Item(i18n.T("chat.fileMenuDefaultBrowser")).Chosen() {
			p.openPreview(path, surface, true)
		}
		if m.Item(i18n.T("chat.fileMenuSource")).Chosen() {
			p.openFile(path, max(target.Line, 1))
		}
	} else {
		if m.Item(i18n.T("chat.fileMenuSidebar")).Chosen() {
			p.openTarget(target, file, surface)
		}
		if !file.Directory && m.Item(i18n.T("chat.openSystem")).Chosen() {
			p.fileAction("file.open", path, surface)
		}
	}
	m.Separator()
	if m.Item(i18n.T("chat.revealFile")).Chosen() {
		p.fileAction("file.reveal", path, surface)
	}
	if !html && !file.Directory && m.Item(i18n.T("chat.fileMenuCopyFile")).Chosen() {
		p.copyFile(path, surface)
	}
	if m.Item(i18n.T("chat.copyPath")).Chosen() {
		p.copy(path)
	}
	if target.Line > 0 && !file.Directory && m.Item(i18n.T("chat.fileMenuCopyLocation")).Chosen() {
		p.copy(locationOf(path, p.directory(), target))
	}
}

// locationOf is path:line[:column], relative to the conversation's
// folder when inside it, to paste into an editor or a terminal.
func locationOf(path, cwd string, target model.LinkTarget) string {
	relative := strings.ReplaceAll(path, `\`, "/")
	root := strings.TrimRight(strings.ReplaceAll(cwd, `\`, "/"), "/") + "/"
	location := path
	if cwd != "" && strings.HasPrefix(strings.ToLower(relative), strings.ToLower(root)) {
		location = relative[len(root):]
	}
	location += ":" + kit.Itoa(target.Line)
	if target.Column > 0 {
		location += ":" + kit.Itoa(target.Column)
	}
	return location
}

// openLink follows a link of a reply, as ChatFileLink: a web address
// opens in the system's browser; a local path is checked with the Host,
// then opens in the side panel (a folder in its file library).
func (p *Chat) openLink(destination, surface string) {
	target := model.TargetOf(destination, p.directory(), p.attachments())
	if !target.Local() {
		if safeLink(destination) {
			p.ui.services.OpenURL(destination)
		}
		return
	}
	id := p.activeID
	if id == "" {
		return
	}
	p.setItemError(surface, "")
	chatCall(p, "file.stat", map[string]any{"id": id, "paths": []string{target.Path}}, func(r struct {
		Files []struct {
			File    *fileStat `json:"file"`
			Missing bool      `json:"missing"`
			Error   string    `json:"error"`
		} `json:"files"`
	}, err error) {
		if err == nil && len(r.Files) == 1 {
			switch f := r.Files[0]; {
			case f.Error != "":
				err = core.Errorf(f.Error)
			case f.Missing || f.File == nil:
				err = core.Errorf(i18n.T("chat.fileLinkMissing"))
			default:
				if p.activeID == id {
					p.openTarget(target, *f.File, surface)
				}
				return
			}
		} else if err == nil {
			err = core.Errorf(i18n.T("chat.fileLinkUnavailable"))
		}
		p.setItemError(surface, err.Error())
	})
}

// answerOptions are an answer's Markdown options: code wraps rather than
// scrolling sideways; links go through openLink, failures shown under the
// answer named key.
func (p *Chat) answerOptions(key string) *markdown.Options {
	s := &p.ui.items
	if s.markdown == nil {
		s.markdown = map[string]*markdown.Options{}
	}
	if o := s.markdown[key]; o != nil {
		// The look follows the window's theme and text size as they change.
		o.Theme = p.ui.reading
		return o
	}
	o := &markdown.Options{Theme: p.ui.reading, WrapCode: true, CodeBlock: kit.CodeBlock, LoadImage: p.markdownImageLoader(key), OnLink: func(destination string) { p.openLink(destination, "answer:"+key) }}
	p.answerCodeBlocks(o, key)
	mathview.For(p.env).Apply(o)
	s.markdown[key] = o
	return o
}
