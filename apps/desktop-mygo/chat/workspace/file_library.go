package workspace

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

type FileEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Size      int64  `json:"size"`
}
type FileListing struct {
	Directory string      `json:"directory"`
	Entries   []FileEntry `json:"entries"`
}
type FileIndex struct {
	Files     []string `json:"files"`
	Truncated bool     `json:"truncated"`
}
type FileMatch struct {
	LineNumber int                        `json:"lineNumber"`
	LineText   string                     `json:"lineText"`
	Submatches []struct{ Start, End int } `json:"submatches"`
}
type SearchFile struct {
	RelPath string      `json:"relPath"`
	Matches []FileMatch `json:"matches"`
}
type SearchResults struct {
	Results      []SearchFile `json:"results"`
	TotalFiles   int          `json:"totalFiles"`
	TotalMatches int          `json:"totalMatches"`
	Truncated    bool         `json:"truncated"`
}
type LibraryOptions struct {
	Prefix, SessionID, Directory string
	OnOpen                       func(path string, line int, newTab bool)
	OnReference                  func(string)
	OnBrowser                    func(string)
}
type FileLibrary struct {
	env                       *core.Env
	options                   LibraryOptions
	ctx                       context.Context
	cancel                    context.CancelFunc
	root, selected, err       string
	entries                   map[string][]FileEntry
	loading                   map[string]bool
	info                      map[string]FileEntry
	outline                   ui.OutlineState[string]
	row                       int
	revisions                 map[string]uint64
	rootRevision              uint64
	index                     FileIndex
	indexLoaded, indexLoading bool
	filter, lastFilter        string
	filtered                  []string
	mode, query               string
	sensitive                 bool
	search                    SearchResults
	searchLoading             bool
	searchRevision            uint64
	searchID                  string
	searchCancel              context.CancelFunc
	timer                     *time.Timer
	shown, closed             bool
	lastFocus                 int
}

func NewFileLibrary(env *core.Env, options LibraryOptions) *FileLibrary {
	ctx, cancel := context.WithCancel(context.Background())
	p := &FileLibrary{env: env, options: options, ctx: ctx, cancel: cancel, root: options.Directory, entries: map[string][]FileEntry{}, info: map[string]FileEntry{}, row: -1, loading: map[string]bool{}, revisions: map[string]uint64{}, mode: "tree", shown: true}
	if env.Focuses != nil {
		p.lastFocus = env.Focuses()
	}
	p.load(options.Directory, true)
	return p
}
func (p *FileLibrary) Error() string { return p.err }
func (p *FileLibrary) Root() string  { return p.root }
func (p *FileLibrary) Close() {
	if p.closed {
		return
	}
	p.closed = true
	p.cancel()
	p.cancelSearch()
}
func (p *FileLibrary) SetVisible(shown bool) {
	if p.closed || p.shown == shown {
		return
	}
	p.shown = shown
	if !shown {
		p.cancelSearch()
	} else if p.mode == "search" && strings.TrimSpace(p.query) != "" {
		p.scheduleSearch()
	}
}
func (p *FileLibrary) load(directory string, root bool) {
	if p.closed || p.loading[directory] {
		return
	}
	p.loading[directory] = true
	p.revisions[directory]++
	revision := p.revisions[directory]
	if root {
		p.rootRevision++
	}
	rootRevision := p.rootRevision
	core.Go(p.env, func() (FileListing, error) {
		var result FileListing
		err := DecodePreviewJSON(p.ctx, p.env, p.options.Prefix, "files.list", map[string]string{"id": p.options.SessionID, "path": directory}, &result)
		return result, err
	}, func(result FileListing, err error) {
		if p.closed || p.revisions[directory] != revision {
			return
		}
		p.loading[directory] = false
		if root && p.rootRevision != rootRevision {
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		p.err = ""
		if root && p.root != result.Directory {
			p.root = result.Directory
			p.indexLoaded = false
			p.filtered = nil
			p.cancelSearch()
			if p.filter != "" {
				p.loadIndex()
			}
		}
		p.entries[result.Directory] = result.Entries
		for _, entry := range result.Entries {
			p.info[entry.Path] = entry
		}
	})
}
func (p *FileLibrary) Refresh() {
	if p.closed {
		return
	}
	p.indexLoaded = false
	p.load(p.root, true)
	for directory := range p.outline.Open.All() {
		p.load(directory, false)
	}
	if p.filter != "" {
		p.loadIndex()
	}
	if p.mode == "search" {
		p.scheduleSearch()
	}
}
func (p *FileLibrary) toggle(directory string) {
	if p.outline.Open.Has(directory) {
		p.outline.Open.Remove(directory)
	} else {
		p.outline.Open.Add(directory)
		if _, loaded := p.entries[directory]; !loaded {
			p.load(directory, false)
		}
	}
}

// Same capped basename-first substring filtering as Cindy filterFiles.ts.
func filterFiles(query string, files []string, limit int) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	names, paths := []string{}, []string{}
	for _, file := range files {
		lower := strings.ToLower(file)
		if !strings.Contains(lower, query) {
			continue
		}
		base := lower[strings.LastIndex(lower, "/")+1:]
		if strings.Contains(base, query) {
			names = append(names, file)
		} else {
			paths = append(paths, file)
		}
		if len(names)+len(paths) >= limit {
			break
		}
	}
	return append(names, paths...)
}
func (p *FileLibrary) loadIndex() {
	if p.closed || p.indexLoading || p.indexLoaded || p.root == "" {
		return
	}
	p.indexLoading = true
	root := p.root
	core.Go(p.env, func() (FileIndex, error) {
		var result FileIndex
		err := DecodePreviewJSON(p.ctx, p.env, p.options.Prefix, "files.index", map[string]string{"id": p.options.SessionID, "path": root}, &result)
		return result, err
	}, func(result FileIndex, err error) {
		if p.closed {
			return
		}
		p.indexLoading = false
		if p.root != root {
			if p.filter != "" {
				p.loadIndex()
			}
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		p.index = result
		p.indexLoaded = true
		p.filtered = filterFiles(p.filter, result.Files, 200)
		p.lastFilter = p.filter
	})
}
func (p *FileLibrary) cancelSearch() {
	if p.timer != nil {
		p.timer.Stop()
	}
	p.searchRevision++
	if p.searchCancel != nil {
		p.searchCancel()
		p.searchCancel = nil
	}
	id := p.searchID
	p.searchID = ""
	p.searchLoading = false
	if id != "" {
		core.Do(p.env, func() error {
			return p.env.Host.CallInto(p.options.Prefix+"files.cancel", map[string]string{"id": p.options.SessionID, "searchId": id}, nil)
		}, func(err error) {
			if !p.closed && err != nil {
				p.err = err.Error()
			}
		})
	}
}
func (p *FileLibrary) scheduleSearch() {
	p.cancelSearch()
	if !p.shown || p.closed || strings.TrimSpace(p.query) == "" || p.root == "" {
		p.search = SearchResults{}
		return
	}
	revision := p.searchRevision
	query, root, sensitive := p.query, p.root, p.sensitive
	p.timer = time.AfterFunc(250*time.Millisecond, func() {
		p.env.Post(func() {
			if p.closed || !p.shown || p.searchRevision != revision {
				return
			}
			ctx, cancel := context.WithCancel(p.ctx)
			p.searchCancel = cancel
			p.searchID = rand.Text()
			p.searchLoading = true
			searchID := p.searchID
			core.Go(p.env, func() (SearchResults, error) {
				var result SearchResults
				err := DecodePreviewJSON(ctx, p.env, p.options.Prefix, "files.search", map[string]any{"id": p.options.SessionID, "path": root, "query": query, "caseSensitive": sensitive, "maxMatches": 500, "searchId": searchID}, &result)
				return result, err
			}, func(result SearchResults, err error) {
				if p.closed || revision != p.searchRevision {
					return
				}
				p.searchLoading = false
				p.searchID = ""
				cancel()
				p.searchCancel = nil
				if err != nil {
					p.err = err.Error()
					return
				}
				p.search = result
				p.err = ""
			})
		})
	})
}
func (p *FileLibrary) open(path string, line int, newTab bool) {
	p.selected = path
	if p.options.OnOpen != nil {
		p.options.OnOpen(path, line, newTab)
	}
}
func (p *FileLibrary) fileAction(method, path string) {
	core.Do(p.env, func() error {
		return p.env.Host.CallInto(p.options.Prefix+method, map[string]string{"id": p.options.SessionID, "path": path}, nil)
	}, func(err error) {
		if !p.closed && err != nil {
			p.err = err.Error()
		}
	})
}
func (p *FileLibrary) View(c *ui.Context) {
	if p.closed {
		return
	}
	if p.env.Focuses != nil {
		focus := p.env.Focuses()
		if focus != p.lastFocus {
			p.lastFocus = focus
			if p.shown {
				p.Refresh()
			}
		}
	}
	root := ui.Column(c).Fill().MinWidth(0)
	root.Children(func() {
		// As the document's header beside it: a name, then tools as icons.
		ui.Row(c).FillWidth().Shrink(0).MinHeight(44).Padding(4, 4, 4, 12).Gap(2).AlignItems(ui.Center).Children(func() {
			// The folder's own name reads in a narrow column; a full path
			// would show as its drive letter and an ellipsis.
			name := filepath.Base(p.root)
			if p.root == "" || name == "." || name == string(filepath.Separator) {
				name = p.root
			}
			ui.Text(c, name).SingleLine().Ellipsis("…").MinWidth(0).Grow(1).Tooltip(p.root)
			search := toolButton(c, icons.Search, wt("search"))
			if p.mode != "tree" {
				search.Background(kit.Colors(c).Selected)
			}
			if search.Clicked() {
				if p.mode == "tree" {
					p.mode = "search"
					p.scheduleSearch()
				} else {
					p.mode = "tree"
					p.cancelSearch()
				}
			}
			if toolButton(c, icons.ChevronsDownUp, wt("collapseAll")).Clicked() {
				p.outline.Open.Clear()
			}
			if toolButton(c, icons.RefreshCw, i18n.T("chat.refresh")).Clicked() {
				p.Refresh()
			}
		})
		ui.Column(c).FillWidth().Grow(1).MinHeight(0).MinWidth(0).Padding(0, 8, 8, 8).Gap(6).Children(func() {
			if p.mode == "tree" {
				p.treeView(c)
			} else {
				p.searchView(c)
			}
			kit.InlineError(c, p.err)
			if p.selected != "" && p.options.OnReference != nil && kit.Button(c, i18n.T("chat.fileReference"), icons.AtSign, false).FillWidth().Justify(ui.Center).Clicked() {
				p.options.OnReference(p.selected)
			}
		})
	})
	p.Drop(c, root)
}
func (p *FileLibrary) treeView(c *ui.Context) {
	if kit.TextInput(c, &p.filter).Label(wt("filter")).Placeholder(wt("filter")).FillWidth().Changed() {
		if p.filter != "" {
			p.loadIndex()
		}
		p.filtered = filterFiles(p.filter, p.index.Files, 200)
		p.lastFilter = p.filter
	}
	if p.filter != "" {
		if p.indexLoading {
			kit.Loader(c)
		}
		ui.List(c, nil, len(p.filtered), func(i int) {
			relative := p.filtered[i]
			path := filepath.Join(p.root, filepath.FromSlash(relative))
			b := kit.GhostButton(c, relative, icons.FileText, false).FillWidth().Justify(ui.Start)
			if b.Clicked() {
				p.open(path, 0, false)
			}
		}).Grow(1).MinHeight(0).FillWidth()
		if p.index.Truncated || len(p.filtered) >= 200 {
			kit.Help(c, wt("truncated"))
		}
		return
	}
	roots := make([]string, 0, len(p.entries[p.root]))
	for _, entry := range p.entries[p.root] {
		roots = append(roots, entry.Path)
	}
	p.outline.List.Selected = &p.row
	p.outline.List.Label = func(i int) string { return p.info[p.outline.Item(i)].Name }
	list := ui.Outline(c, &p.outline, roots, func(path string) []string {
		if !p.info[path].Directory {
			return nil
		}
		children, loaded := p.entries[path]
		if !loaded && p.outline.Open.Has(path) {
			p.load(path, false)
		}
		keys := make([]string, 0, len(children))
		for _, child := range children {
			keys = append(keys, child.Path)
		}
		return keys
	}, func(path string) {
		entry := p.info[path]
		row := ui.Row(c).FillWidth().MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
			icon := icons.FileText
			if entry.Directory {
				icon = icons.Folder
			}
			ui.Icon(c, icon).FontSize(14).TextColor(c.Theme().TextMuted).Shrink(0)
			ui.Text(c, entry.Name).SingleLine().Ellipsis("…").Grow(1).MinWidth(0).Tooltip(entry.Path)
			if p.loading[path] {
				kit.Loader(c)
			}
		})
		row.ContextMenu(func(m *ui.Menu) {
			if m.Item(i18n.T("chat.copyPath")).Chosen() {
				c.Services().WriteClipboard(path)
			}
			if m.Item(i18n.T("chat.revealFile")).Chosen() {
				p.fileAction("file.reveal", path)
			}
			if m.Item(i18n.T("chat.openSystem")).Chosen() {
				p.fileAction("file.open", path)
			}
			if !entry.Directory && m.Item(wt("openTab")).Chosen() {
				p.open(path, 0, true)
			}
		})
	}).Grow(1).MinHeight(0).FillWidth()
	changed, submitted := list.Changed(), list.Submitted()
	if (changed || submitted) && p.row >= 0 && p.row < p.outline.Rows() {
		path := p.outline.Item(p.row)
		if p.info[path].Directory {
			if submitted {
				p.toggle(path)
			}
		} else {
			p.open(path, 0, false)
		}
	}
}
func (p *FileLibrary) searchView(c *ui.Context) {
	if kit.TextInput(c, &p.query).Label(wt("search")).Placeholder(wt("search")).FillWidth().Changed() {
		p.scheduleSearch()
	}
	// A check box: a setting row's height would crowd a narrow column.
	if ui.Checkbox(c, &p.sensitive, wt("caseSensitive")).Changed() {
		p.scheduleSearch()
	}
	if p.searchLoading {
		kit.Loader(c)
	} else if p.search.TotalMatches > 0 {
		kit.Help(c, wt("matches", "files", p.search.TotalFiles, "count", p.search.TotalMatches))
	}
	type resultRow struct {
		path string
		line FileMatch
	}
	rows := []resultRow{}
	for _, file := range p.search.Results {
		for _, line := range file.Matches {
			rows = append(rows, resultRow{file.RelPath, line})
		}
	}
	ui.List(c, nil, len(rows), func(i int) {
		row := rows[i]
		b := ui.ButtonBase(c).FillWidth().MinWidth(0).Padding(6, 8).Radius(c.Theme().Radius).Label(row.path)
		if b.Hovered() {
			b.Background(c.Theme().SurfaceHover)
		}
		b.Children(func() {
			ui.Column(c).FillWidth().MinWidth(0).Gap(2).Children(func() {
				kit.Caption(c, row.path).SingleLine().Ellipsis("…").Tooltip(row.path)
				kit.Mono(c, strings.TrimSpace(row.line.LineText)).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(row.line.LineText)
			})
		})
		if b.Clicked() {
			p.open(filepath.Join(p.root, filepath.FromSlash(row.path)), row.line.LineNumber, false)
		}
	}).Grow(1).MinHeight(0).FillWidth()
	if p.search.Truncated {
		kit.Help(c, wt("truncated"))
	}
}

// A native drop is inspected by the Host before deciding whether to browse a
// directory or open a document; the UI never treats an outside path as a root.
func (p *FileLibrary) Drop(c *ui.Context, target ui.Element) {
	widgets.DropArea(c, p.env, target, widgets.DropAny, false, false, func(paths []string, err error) {
		if p.closed {
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		if len(paths) == 0 {
			return
		}
		path := paths[0]
		core.Go(p.env, func() (json.RawMessage, error) {
			return p.env.Host.Call(p.options.Prefix+"file.stat", map[string]any{"id": p.options.SessionID, "paths": []string{path}})
		}, func(raw json.RawMessage, err error) {
			if p.closed {
				return
			}
			if err != nil {
				p.err = err.Error()
				return
			}
			var response struct {
				Files []struct {
					File    *FileEntry `json:"file"`
					Error   string     `json:"error"`
					Missing bool       `json:"missing"`
				} `json:"files"`
			}
			if err = json.Unmarshal(raw, &response); err != nil {
				p.err = err.Error()
				return
			}
			if len(response.Files) != 1 || response.Files[0].File == nil {
				p.err = i18n.T("chat.fileLinkMissing")
				if len(response.Files) == 1 && response.Files[0].Error != "" {
					p.err = response.Files[0].Error
				}
				return
			}
			if response.Files[0].File.Directory {
				p.load(path, true)
			} else {
				p.open(path, 0, false)
			}
		})
	})
}
func wt(key string, args ...any) string { return i18n.T("nativeWorkspace."+key, args...) }
