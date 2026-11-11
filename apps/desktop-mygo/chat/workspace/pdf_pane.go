package workspace

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/pdfium"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type PDFOptions struct {
	InitialPreview                  *FilePreview
	Prefix, SessionID, Path, Worker string
	OnLink                          func(string)
	Preview                         func() (FilePreview, error)
}
type pdfMatch struct{ page, start, end int }
type pdfFieldDraft struct {
	field   pdfium.Field
	value   string
	checked bool
	option  string
}

// PDFPane draws native controls and the original engine's page. Text geometry
// comes from PDFium, not from reflowing or guessing the document's layout.
type PDFPane struct {
	env                                                     *core.Env
	options                                                 PDFOptions
	ctx                                                     context.Context
	cancel                                                  context.CancelFunc
	client                                                  *pdfium.Client
	count, index, renderedIndex, rotation, renderedRotation int
	page                                                    pdfium.Page
	bitmap                                                  *ui.Bitmap
	viewport, pageBox                                       ui.Handle
	width                                                   float64
	scale                                                   float64
	renderScale                                             float64
	pageNumber                                              float64
	rendering, renderAgain, loading, closed                 bool
	err                                                     string
	password                                                string
	passwordOpen                                            bool
	data                                                    []byte
	query                                                   string
	searchLoading                                           bool
	searchRevision                                          uint64
	matches                                                 []pdfMatch
	matchIndex                                              int
	anchor, selectionEnd                                    int
	selecting                                               bool
	formOpen, formBusy                                      bool
	forms                                                   map[int]*pdfFieldDraft
	panX                                                    float32
}

func NewPDFPane(env *core.Env, options PDFOptions) *PDFPane {
	ctx, cancel := context.WithCancel(context.Background())
	p := &PDFPane{env: env, options: options, ctx: ctx, cancel: cancel, scale: 1, pageNumber: 1, anchor: -1, selectionEnd: -1, matchIndex: -1, renderedIndex: -1, forms: map[int]*pdfFieldDraft{}}
	p.open()
	return p
}
func (p *PDFPane) Busy() bool    { return p.formBusy }
func (p *PDFPane) Error() string { return p.err }
func (p *PDFPane) Close() {
	if p.closed {
		return
	}
	p.closed = true
	p.cancel()
	p.searchRevision++
	if p.client != nil {
		client := p.client
		p.client = nil
		core.Do(p.env, client.Close, func(err error) {
			if err != nil && p.env.Toast != nil {
				p.env.Toast(err.Error())
			}
		})
	}
	p.bitmap = nil
	p.data = nil
}
func (p *PDFPane) open() {
	if p.loading || p.closed {
		return
	}
	p.loading = true
	p.err = ""
	worker := p.options.Worker
	if worker == "" {
		worker = os.Getenv("WINONE_PDF_WORKER")
	}
	if worker == "" {
		executable, err := os.Executable()
		if err != nil {
			p.loading = false
			p.err = err.Error()
			return
		}
		worker = filepath.Join(filepath.Dir(executable), "WinOne.PDF.exe")
	}
	data := p.data
	password := p.password
	client := p.client
	type opened struct {
		client *pdfium.Client
		data   []byte
		count  int
	}
	core.Go(p.env, func() (opened, error) {
		result := opened{client: client, data: data}
		if len(data) == 0 {
			var preview FilePreview
			if p.options.Preview != nil {
				var err error
				preview, err = p.options.Preview()
				if err != nil {
					return result, err
				}
			} else if p.options.InitialPreview != nil {
				preview = *p.options.InitialPreview
			} else if err := p.env.Host.CallInto(p.options.Prefix+"file.preview", map[string]string{"id": p.options.SessionID, "path": p.options.Path}, &preview); err != nil {
				return result, err
			}
			var err error
			result.data, err = FetchPreview(p.ctx, preview.URL, pdfium.MaxDocumentBytes)
			if err != nil {
				return result, err
			}
		}
		if result.client == nil {
			var err error
			result.client, err = pdfium.Start(p.ctx, worker, filepath.Join(filepath.Dir(worker), "pdfium.dll"))
			if err != nil {
				return result, err
			}
		}
		response, err := result.client.Call(p.ctx, pdfium.Request{Method: "open", Data: result.data, Password: password})
		result.count = response.Count
		return result, err
	}, func(result opened, err error) {
		if p.closed {
			if result.client != nil {
				core.Do(p.env, result.client.Close, func(error) {})
			}
			return
		}
		p.loading = false
		p.client = result.client
		p.data = result.data
		if err != nil {
			p.err = wt("pdfReadFailed") + ": " + err.Error()
			if strings.Contains(err.Error(), "password") {
				p.passwordOpen = true
			}
			return
		}
		p.password = ""
		p.passwordOpen = false
		p.data = nil
		p.count = result.count
		p.index = 0
		p.render()
	})
}
func (p *PDFPane) render() {
	if p.closed || p.client == nil || p.count == 0 {
		return
	}
	if p.rendering {
		p.renderAgain = true
		return
	}
	p.rendering = true
	index, rotation, scale := p.index, p.rotation, p.scale
	width := p.width
	if width <= 0 {
		width = 700
	}
	client := p.client
	type rendered struct {
		page   pdfium.Page
		bitmap *ui.Bitmap
	}
	core.Go(p.env, func() (rendered, error) {
		var result rendered
		response, err := client.Call(p.ctx, pdfium.Request{Method: "page", Page: index})
		if err != nil {
			return result, err
		}
		if response.Page == nil {
			return result, fmt.Errorf("%s", wt("pdfReadFailed"))
		}
		result.page = *response.Page
		pageWidth, pageHeight := result.page.Width, result.page.Height
		if rotation%2 == 1 {
			pageWidth, pageHeight = pageHeight, pageWidth
		}
		pixelWidth := width * scale
		pixelHeight := pixelWidth * pageHeight / pageWidth
		if pixelWidth*pixelHeight > 8*1024*1024 {
			factor := math.Sqrt(8 * 1024 * 1024 / (pixelWidth * pixelHeight))
			pixelWidth *= factor
			pixelHeight *= factor
		}
		response, err = client.Call(p.ctx, pdfium.Request{Method: "render", Page: index, Width: max(1, int(pixelWidth)), Height: max(1, int(pixelHeight)), Rotation: rotation})
		if err != nil {
			return result, err
		}
		result.bitmap, err = ui.DecodeBitmap(response.PNG)
		return result, err
	}, func(result rendered, err error) {
		if p.closed {
			return
		}
		p.rendering = false
		if err != nil {
			p.err = wt("pdfReadFailed") + ": " + err.Error()
		} else if index == p.index && rotation == p.rotation && scale == p.scale {
			p.page, p.bitmap = result.page, result.bitmap
			p.renderedIndex, p.renderedRotation, p.renderScale = index, rotation, scale
			p.pageNumber = float64(index + 1)
			p.anchor, p.selectionEnd = -1, -1
			if p.matchIndex >= 0 && p.matchIndex < len(p.matches) && p.matches[p.matchIndex].page == index {
				match := p.matches[p.matchIndex]
				p.anchor, p.selectionEnd = match.start, match.end
			}
			p.err = ""
		}
		if p.renderAgain {
			p.renderAgain = false
			p.render()
		}
	})
}
func (p *PDFPane) setPage(index int) {
	if index < 0 || index >= p.count || p.index == index {
		return
	}
	p.index = index
	p.pageNumber = float64(index + 1)
	p.render()
}
func (p *PDFPane) SelectedText() string {
	if p.anchor < 0 || p.selectionEnd < 0 || len(p.page.Characters) == 0 {
		return ""
	}
	first, last := min(p.anchor, p.selectionEnd), max(p.anchor, p.selectionEnd)
	first = max(0, first)
	last = min(len(p.page.Characters)-1, last)
	var text strings.Builder
	for _, character := range p.page.Characters[first : last+1] {
		text.WriteString(character.Text)
	}
	return text.String()
}
func (p *PDFPane) find() {
	if p.searchLoading || p.closed || p.client == nil {
		return
	}
	query := strings.ToLower(p.query)
	if query == "" {
		p.matches = nil
		return
	}
	p.searchLoading = true
	p.searchRevision++
	revision := p.searchRevision
	count, client := p.count, p.client
	core.Go(p.env, func() ([]pdfMatch, error) {
		result := []pdfMatch{}
		for page := 0; page < count; page++ {
			response, err := client.Call(p.ctx, pdfium.Request{Method: "page", Page: page})
			if err != nil {
				return nil, err
			}
			if response.Page == nil {
				continue
			}
			matches := findPDFText(*response.Page, query)
			for _, match := range matches {
				result = append(result, pdfMatch{page, match.start, match.end})
				if len(result) >= 2000 {
					return result, nil
				}
			}
		}
		return result, nil
	}, func(result []pdfMatch, err error) {
		if p.closed || revision != p.searchRevision {
			return
		}
		p.searchLoading = false
		if strings.ToLower(p.query) != query {
			return
		}
		if err != nil {
			p.err = wt("pdfReadFailed") + ": " + err.Error()
			return
		}
		p.matches = result
		p.matchIndex = -1
		p.nextMatch(1)
	})
}
func (p *PDFPane) nextMatch(delta int) {
	if len(p.matches) == 0 {
		return
	}
	p.matchIndex = (p.matchIndex + delta + len(p.matches)) % len(p.matches)
	match := p.matches[p.matchIndex]
	p.setPage(match.page)
	if p.renderedIndex == match.page {
		p.anchor, p.selectionEnd = match.start, match.end
	}
}
func pdfPoint(x, y, width, height float64, rotation int) (float64, float64) {
	switch rotation % 4 {
	case 1:
		return height - y, x
	case 2:
		return width - x, height - y
	case 3:
		return y, width - x
	}
	return x, y
}
func (p *PDFPane) box(box pdfium.Box, bounds ui.Rect) ui.Rect {
	width, height := p.page.Width, p.page.Height
	rotWidth, rotHeight := width, height
	if p.renderedRotation%2 == 1 {
		rotWidth, rotHeight = height, width
	}
	x1, y1 := pdfPoint(box.Left, box.Top, width, height, p.renderedRotation)
	x2, y2 := pdfPoint(box.Right, box.Bottom, width, height, p.renderedRotation)
	return ui.Rect{X: bounds.X + float32(math.Min(x1, x2)/rotWidth)*bounds.W, Y: bounds.Y + float32(math.Min(y1, y2)/rotHeight)*bounds.H, W: float32(math.Abs(x2-x1)/rotWidth) * bounds.W, H: float32(math.Abs(y2-y1)/rotHeight) * bounds.H}
}
func (p *PDFPane) nearest(x, y float32, bounds ui.Rect) int {
	best := float64(math.MaxFloat64)
	index := -1
	for i, character := range p.page.Characters {
		r := p.box(character.Box, bounds)
		if r.W <= 0 || r.H <= 0 {
			continue
		}
		dx := float64(max(r.X-x, max(0, x-(r.X+r.W))))
		dy := float64(max(r.Y-y, max(0, y-(r.Y+r.H))))
		distance := dx*dx + dy*dy
		if distance < best {
			best = distance
			index = i
		}
	}
	return index
}
func (p *PDFPane) openForms() {
	p.forms = map[int]*pdfFieldDraft{}
	for _, field := range p.page.Fields {
		p.forms[field.Index] = &pdfFieldDraft{field: field, value: field.Value, checked: field.Checked, option: field.Value}
	}
	p.formOpen = true
}
func (p *PDFPane) applyForms() {
	if p.formBusy || p.closed {
		return
	}
	p.formBusy = true
	requests := []pdfium.Request{}
	for _, draft := range p.forms {
		field := draft.field
		if field.ReadOnly {
			continue
		}
		request := pdfium.Request{Method: "form", Page: p.renderedIndex, Annotation: field.Index, Value: draft.value}
		if field.Kind == pdfium.FieldCheckbox || field.Kind == pdfium.FieldRadio {
			if draft.checked == field.Checked {
				continue
			}
			checked := draft.checked
			request.Checked = &checked
		} else if field.Kind == pdfium.FieldList || field.Kind == pdfium.FieldCombo {
			if draft.option == field.Value {
				continue
			}
			for index, value := range field.Options {
				if value == draft.option {
					option := index
					request.Option = &option
				}
			}
		} else if draft.value == field.Value {
			continue
		}
		requests = append(requests, request)
	}
	client := p.client
	core.Do(p.env, func() error {
		for _, request := range requests {
			if _, err := client.Call(p.ctx, request); err != nil {
				return err
			}
		}
		return nil
	}, func(err error) {
		if p.closed {
			return
		}
		p.formBusy = false
		if err != nil {
			p.err = wt("pdfFormFailed") + ": " + err.Error()
			return
		}
		p.formOpen = false
		p.render()
	})
}
func (p *PDFPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	// Use committed native bounds only while the Context is valid.
	width := float64(p.viewport.Bounds(c).W)
	if width > 0 && math.Abs(width-p.width) > 1 {
		p.width = width
		if p.bitmap != nil {
			p.render()
		}
	}
	root := ui.Column(c).Fill().MinWidth(0).Gap(4)
	root.Children(func() {
		ui.Row(c).FillWidth().Gap(4).Padding(8).Wrap().Children(func() {
			if kit.IconButton(c, icons.ChevronLeft, i18n.T("chat.pdfPrevious")).Disabled(p.index == 0 || p.loading).Clicked() {
				p.setPage(p.index - 1)
			}
			if kit.IconButton(c, icons.ChevronRight, i18n.T("chat.pdfNext")).Disabled(p.index >= p.count-1 || p.loading).Clicked() {
				p.setPage(p.index + 1)
			}
			if p.count > 0 {
				if ui.NumberInput(c, &p.pageNumber, 1, float64(p.count), 1).Width(95).Label(wt("pdfPageNumber")).Changed() {
					p.setPage(int(p.pageNumber) - 1)
				}
				kit.Help(c, fmt.Sprintf("/ %d", p.count))
			}
			if kit.IconButton(c, icons.ZoomIn, i18n.T("chat.zoomIn")).Clicked() {
				p.scale = math.Min(10, p.scale*1.25)
				p.render()
			}
			if kit.IconButton(c, icons.ZoomOut, i18n.T("chat.zoomOut")).Clicked() {
				p.scale = math.Max(.1, p.scale/1.25)
				p.render()
			}
			if kit.IconButton(c, icons.Maximize, i18n.T("chat.pdfFit")).Clicked() {
				p.scale = 1
				p.panX = 0
				p.render()
			}
			if kit.IconButton(c, icons.RotateCw, i18n.T("chat.pdfRotate")).Clicked() {
				p.rotation = (p.rotation + 1) % 4
				p.render()
			}
			if len(p.page.Fields) > 0 && kit.Button(c, wt("pdfForms"), icons.SquarePen, false).Clicked() {
				p.openForms()
			}
			if kit.IconButton(c, icons.Copy, i18n.T("chat.copy")).Disabled(p.SelectedText() == "").Clicked() {
				c.Services().WriteClipboard(p.SelectedText())
			}
		})
		ui.Row(c).FillWidth().Gap(4).Padding(0, 8).Children(func() {
			input := kit.TextInput(c, &p.query).Grow(1).MinWidth(0).Placeholder(i18n.T("chat.pdfFind")).Label(i18n.T("chat.pdfFind"))
			if kit.Button(c, i18n.T("chat.pdfFind"), icons.Search, p.searchLoading).Disabled(p.client == nil).Clicked() || input.Shortcut(0, ui.KeyEnter) {
				p.find()
			}
			if len(p.matches) > 0 {
				if kit.IconButton(c, icons.ChevronUp, wt("pdfPreviousMatch")).Clicked() {
					p.nextMatch(-1)
				}
				if kit.IconButton(c, icons.ChevronDown, wt("pdfNextMatch")).Clicked() {
					p.nextMatch(1)
				}
			}
		})
		if len(p.matches) > 0 {
			kit.Help(c, wt("pdfMatches", "current", p.matchIndex+1, "count", len(p.matches)))
		}
		kit.InlineError(c, p.err)
		if p.loading || p.rendering {
			kit.Loader(c)
		}
		ui.Scroll(c).Grow(1).MinHeight(0).FillWidth().Bind(&p.viewport).ClipX().Children(func() {
			if p.bitmap == nil {
				return
			}
			width, height := p.bitmap.Size()
			displayWidth := p.viewport.Bounds(c).W
			if displayWidth <= 0 {
				displayWidth = float32(width)
			}
			displayWidth *= float32(p.scale)
			stage := ui.Box(c).Width(displayWidth).Height(displayWidth * float32(height) / float32(width)).Left(p.panX).Focusable().Cursor(ui.CursorText).Bind(&p.pageBox)
			stage.Children(func() {
				ui.Image(c, p.bitmap).Fill().Fit(ui.Contain)
				ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).PassThrough().Draw(func(painter *ui.Painter, r ui.Rect) {
					for _, box := range p.selectionRects(r) {
						painter.Fill(box, c.Theme().Accent.Alpha(.22), 0)
					}
				})
				localBounds := ui.Rect{W: displayWidth, H: displayWidth * float32(height) / float32(width)}
				for _, link := range p.page.Links {
					r := p.box(link.Box, localBounds)
					if r.W <= 0 || r.H <= 0 {
						continue
					}
					button := ui.Box(c).Absolute().Left(r.X).Top(r.Y).Width(r.W).Height(r.H).Focusable().Role(ui.RoleLink).Label(link.URL)
					if button.Clicked() {
						if link.Page >= 0 {
							p.setPage(link.Page)
						} else if p.options.OnLink != nil {
							p.options.OnLink(link.URL)
						}
					}
				}
			})
			bounds := p.pageBox.Bounds(c)
			x, y, inside := stage.PointerPosition()
			if stage.Pressed() && !p.selecting && inside && c.Modifiers()&ui.Alt == 0 {
				p.anchor = p.nearest(bounds.X+x, bounds.Y+y, bounds)
				p.selectionEnd = p.anchor
				p.selecting = true
			}
			if dx, _, dragged := stage.Dragged(); dragged {
				if c.Modifiers()&ui.Alt != 0 {
					p.panX = min(float32(0), max(p.viewport.Bounds(c).W-displayWidth, p.panX+dx))
				} else if p.selecting {
					p.selectionEnd = p.nearest(bounds.X+x, bounds.Y+y, bounds)
				}
			}
			if _, _, ended := stage.DragEnded(); ended || stage.Clicked() {
				p.selecting = false
			}
			stage.ContextMenu(func(m *ui.Menu) {
				if m.Item(i18n.T("chat.copy")).Chosen() {
					c.Services().WriteClipboard(p.SelectedText())
				}
				if m.Item(wt("pdfSelectAll")).Chosen() {
					p.anchor, p.selectionEnd = 0, len(p.page.Characters)-1
				}
			})
		})
	})
	if root.Shortcut(ui.Ctrl, ui.KeyC) {
		c.Services().WriteClipboard(p.SelectedText())
	}
	if p.passwordOpen {
		if kit.Dialog(c, &p.passwordOpen, kit.DialogOptions{Title: wt("pdfPassword"), Confirm: wt("pdfOpen"), Cancel: i18n.T("chat.cancel"), Busy: p.loading, Error: p.err, Body: func() { kit.TextInput(c, &p.password).Label(wt("pdfPassword")).Password().FillWidth() }}) {
			p.open()
		}
	}
	if p.formOpen {
		if kit.Dialog(c, &p.formOpen, kit.DialogOptions{Title: wt("pdfForms"), Confirm: wt("pdfApplyForms"), Cancel: i18n.T("chat.cancel"), Busy: p.formBusy, Error: p.err, Width: 650, Body: func() {
			for _, field := range p.page.Fields {
				draft := p.forms[field.Index]
				if draft == nil {
					continue
				}
				label := field.Name
				if label == "" {
					label = wt("pdfField", "index", field.Index+1)
				}
				ui.Field(c, label, func() {
					switch field.Kind {
					case pdfium.FieldCheckbox, pdfium.FieldRadio:
						ui.Checkbox(c, &draft.checked, label).Disabled(field.ReadOnly || p.formBusy)
					case pdfium.FieldList, pdfium.FieldCombo:
						options := []kit.SelectOption{}
						for _, value := range field.Options {
							options = append(options, kit.SelectOption{Value: value, Label: value})
						}
						_, trigger := kit.Select(c, &draft.option, options)
						trigger.Disabled(field.ReadOnly || p.formBusy)
					case pdfium.FieldText:
						kit.TextInput(c, &draft.value).FillWidth().Disabled(field.ReadOnly || p.formBusy)
					default:
						kit.Help(c, field.Value)
					}
				})
			}
		}}) {
			p.applyForms()
		}
	}
}
