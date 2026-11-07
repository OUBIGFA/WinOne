//go:build windows

// Package pdfium is a narrow binding to the reviewed PDFium C ABI. Rendering,
// text geometry, links and AcroForm semantics are owned by the original engine;
// production calls it only in WinOne.PDF.exe, not in the MyGo window process.
package pdfium

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"runtime"
	"sync"
	"unicode/utf16"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/windows"
)

const LibrarySHA256 = "a318c39c137482dc4687c2973b7ec1c27fc1c20253cf2dc31928f922eab89e8f"
const MaxDocumentBytes = 128 << 20

// The exact field type definitions come from the reviewed 8111 header.
const (
	FieldUnknown   = 0
	FieldButton    = 1
	FieldCheckbox  = 2
	FieldRadio     = 3
	FieldCombo     = 4
	FieldList      = 5
	FieldText      = 6
	FieldSignature = 7
)

type Box struct{ Left, Top, Right, Bottom float64 }
type Character struct {
	Text string
	Box  Box
}
type Link struct {
	Box  Box
	URL  string
	Page int
}
type Field struct {
	Index             int
	Name, Value       string
	Kind              int
	Box               Box
	ReadOnly, Checked bool
	Options           []string
}
type Page struct {
	Index         int
	Width, Height float64
	Characters    []Character
	Links         []Link
	Fields        []Field
}
type api struct {
	init                  func()
	destroy               func()
	load                  func(unsafe.Pointer, uintptr, string) uintptr
	closeDoc              func(uintptr)
	count                 func(uintptr) int32
	loadPage              func(uintptr, int32) uintptr
	closePage             func(uintptr)
	width, height         func(uintptr) float32
	bitmap                func(int32, int32, int32, unsafe.Pointer, int32) uintptr
	fill                  func(uintptr, int32, int32, int32, int32, uint32) int32
	render                func(uintptr, uintptr, int32, int32, int32, int32, int32, int32)
	destroyBitmap         func(uintptr)
	loadText              func(uintptr) uintptr
	closeText             func(uintptr)
	charCount             func(uintptr) int32
	unicode               func(uintptr, int32) uint32
	charBox               func(uintptr, int32, *float64, *float64, *float64, *float64) int32
	enumerateLink         func(uintptr, *int32, *uintptr) int32
	linkRect              func(uintptr, *rect) int32
	linkAction            func(uintptr) uintptr
	actionType            func(uintptr) uint32
	uri                   func(uintptr, uintptr, unsafe.Pointer, uint32) uint32
	linkDest              func(uintptr, uintptr) uintptr
	destPage              func(uintptr, uintptr) int32
	initForm              func(uintptr, *formInfo) uintptr
	closeForm             func(uintptr)
	afterLoad             func(uintptr, uintptr)
	beforeClose           func(uintptr, uintptr)
	drawForm              func(uintptr, uintptr, uintptr, int32, int32, int32, int32, int32, int32)
	annotCount            func(uintptr) int32
	annot                 func(uintptr, int32) uintptr
	closeAnnot            func(uintptr)
	annotRect             func(uintptr, *rect) int32
	fieldType, fieldFlags func(uintptr, uintptr) int32
	fieldName, fieldValue func(uintptr, uintptr, unsafe.Pointer, uint32) uint32
	optionCount           func(uintptr, uintptr) int32
	optionLabel           func(uintptr, uintptr, int32, unsafe.Pointer, uint32) uint32
	checked               func(uintptr, uintptr) int32
	killFocus             func(uintptr) int32
	focus                 func(uintptr, uintptr) int32
	keyDown               func(uintptr, uintptr, int32, int32) int32
	replace               func(uintptr, uintptr, unsafe.Pointer)
	clickDown, clickUp    func(uintptr, uintptr, int32, float64, float64) int32
	setOption             func(uintptr, uintptr, int32, int32) int32
}
type rect struct{ Left, Top, Right, Bottom float32 }

// Stable version-1 FPDF_FORMFILLINFO: one aligned int followed by its 16
// callbacks and a JS platform pointer. No V8/XFA or external action callbacks.
type formInfo struct {
	Version   int32
	Padding   int32
	Callbacks [16]uintptr
}
type Engine struct {
	mu                             sync.Mutex
	dll                            windows.Handle
	api                            api
	document, page, textPage, form uintptr
	pageIndex                      int
	data                           []byte
	pin                            runtime.Pinner
	formPin                        runtime.Pinner
	info                           formInfo
	closed                         bool
}

func Load(library string) (engine *Engine, err error) {
	file, err := os.Open(library)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	_, readErr := io.Copy(hash, file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if hex.EncodeToString(hash.Sum(nil)) != LibrarySHA256 {
		return nil, errors.New("PDFium library checksum mismatch")
	}
	dll, err := windows.LoadLibraryEx(library, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return nil, err
	}
	e := &Engine{dll: dll, pageIndex: -1}
	// Symbol errors fail before initialization; never continue with a partial ABI.
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = windows.FreeLibrary(dll)
			engine = nil
			err = fmt.Errorf("PDFium ABI: %v", recovered)
		}
	}()
	bind := func(name string, target any) {
		symbol, err := windows.GetProcAddress(dll, name)
		if err != nil {
			panic(err)
		}
		purego.RegisterFunc(target, symbol)
	}
	a := &e.api
	bind("FPDF_InitLibrary", &a.init)
	bind("FPDF_DestroyLibrary", &a.destroy)
	bind("FPDF_LoadMemDocument64", &a.load)
	bind("FPDF_CloseDocument", &a.closeDoc)
	bind("FPDF_GetPageCount", &a.count)
	bind("FPDF_LoadPage", &a.loadPage)
	bind("FPDF_ClosePage", &a.closePage)
	bind("FPDF_GetPageWidthF", &a.width)
	bind("FPDF_GetPageHeightF", &a.height)
	bind("FPDFBitmap_CreateEx", &a.bitmap)
	bind("FPDFBitmap_FillRect", &a.fill)
	bind("FPDF_RenderPageBitmap", &a.render)
	bind("FPDFBitmap_Destroy", &a.destroyBitmap)
	bind("FPDFText_LoadPage", &a.loadText)
	bind("FPDFText_ClosePage", &a.closeText)
	bind("FPDFText_CountChars", &a.charCount)
	bind("FPDFText_GetUnicode", &a.unicode)
	bind("FPDFText_GetCharBox", &a.charBox)
	bind("FPDFLink_Enumerate", &a.enumerateLink)
	bind("FPDFLink_GetAnnotRect", &a.linkRect)
	bind("FPDFLink_GetAction", &a.linkAction)
	bind("FPDFAction_GetType", &a.actionType)
	bind("FPDFAction_GetURIPath", &a.uri)
	bind("FPDFLink_GetDest", &a.linkDest)
	bind("FPDFDest_GetDestPageIndex", &a.destPage)
	bind("FPDFDOC_InitFormFillEnvironment", &a.initForm)
	bind("FPDFDOC_ExitFormFillEnvironment", &a.closeForm)
	bind("FORM_OnAfterLoadPage", &a.afterLoad)
	bind("FORM_OnBeforeClosePage", &a.beforeClose)
	bind("FPDF_FFLDraw", &a.drawForm)
	bind("FPDFPage_GetAnnotCount", &a.annotCount)
	bind("FPDFPage_GetAnnot", &a.annot)
	bind("FPDFPage_CloseAnnot", &a.closeAnnot)
	bind("FPDFAnnot_GetRect", &a.annotRect)
	bind("FPDFAnnot_GetFormFieldType", &a.fieldType)
	bind("FPDFAnnot_GetFormFieldFlags", &a.fieldFlags)
	bind("FPDFAnnot_GetFormFieldName", &a.fieldName)
	bind("FPDFAnnot_GetFormFieldValue", &a.fieldValue)
	bind("FPDFAnnot_GetOptionCount", &a.optionCount)
	bind("FPDFAnnot_GetOptionLabel", &a.optionLabel)
	bind("FPDFAnnot_IsChecked", &a.checked)
	bind("FORM_ForceToKillFocus", &a.killFocus)
	bind("FORM_SetFocusedAnnot", &a.focus)
	bind("FORM_OnKeyDown", &a.keyDown)
	bind("FORM_ReplaceSelection", &a.replace)
	bind("FORM_OnLButtonDown", &a.clickDown)
	bind("FORM_OnLButtonUp", &a.clickUp)
	bind("FORM_SetIndexSelected", &a.setOption)
	a.init()
	e.initFormInfo()
	return e, nil
}
func (e *Engine) Open(data []byte, password string) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return 0, errors.New("PDFium closed")
	}
	e.closeDocument()
	if len(data) == 0 || len(data) > MaxDocumentBytes {
		return 0, errors.New("PDF document size is invalid")
	}
	e.data = append([]byte(nil), data...)
	e.pin.Pin(&e.data[0])
	e.document = e.api.load(unsafe.Pointer(&e.data[0]), uintptr(len(e.data)), password)
	if e.document == 0 {
		e.pin.Unpin()
		e.data = nil
		return 0, errors.New("PDF could not be opened; verify the document or password")
	}
	e.formPin.Pin(&e.info)
	e.form = e.api.initForm(e.document, &e.info)
	return int(e.api.count(e.document)), nil
}
func (e *Engine) initFormInfo() {
	// Allocate the stable callbacks once per helper, not once per password
	// attempt/document reload: Windows callbacks are process-lifetime objects.
	// External actions and JavaScript have no host callback and cannot run.
	e.info = formInfo{Version: 1}
	e.info.Callbacks[8] = windows.NewCallback(func(_ uintptr, document uintptr, index int32) uintptr {
		if document == e.document && int(index) == e.pageIndex {
			return e.page
		}
		return 0
	})
	e.info.Callbacks[9] = windows.NewCallback(func(_ uintptr, document uintptr) uintptr {
		if document == e.document {
			return e.page
		}
		return 0
	})
	e.info.Callbacks[10] = windows.NewCallback(func(_, _ uintptr) uintptr { return 0 })
}

func (e *Engine) closePage() {
	if e.textPage != 0 {
		e.api.closeText(e.textPage)
		e.textPage = 0
	}
	if e.page != 0 {
		if e.form != 0 {
			e.api.beforeClose(e.page, e.form)
		}
		e.api.closePage(e.page)
		e.page = 0
	}
	e.pageIndex = -1
}
func (e *Engine) closeDocument() {
	e.closePage()
	if e.form != 0 {
		e.api.closeForm(e.form)
		e.form = 0
	}
	e.formPin.Unpin()
	if e.document != 0 {
		e.api.closeDoc(e.document)
		e.document = 0
	}
	e.pin.Unpin()
	e.data = nil
}
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	e.closeDocument()
	e.api.destroy()
	return windows.FreeLibrary(e.dll)
}
func (e *Engine) ensurePage(index int) error {
	if e.closed || e.document == 0 {
		return errors.New("PDF not open")
	}
	if index < 0 || index >= int(e.api.count(e.document)) {
		return errors.New("PDF page index out of range")
	}
	if e.pageIndex == index {
		return nil
	}
	e.closePage()
	e.page = e.api.loadPage(e.document, int32(index))
	if e.page == 0 {
		return errors.New("PDF page could not be loaded")
	}
	e.pageIndex = index
	if e.form != 0 {
		e.api.afterLoad(e.page, e.form)
	}
	e.textPage = e.api.loadText(e.page)
	return nil
}
func convertedRect(r rect, height float64) Box {
	return Box{float64(r.Left), height - float64(r.Top), float64(r.Right), height - float64(r.Bottom)}
}
func (e *Engine) Page(index int) (Page, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ensurePage(index); err != nil {
		return Page{}, err
	}
	a := &e.api
	result := Page{Index: index, Width: float64(a.width(e.page)), Height: float64(a.height(e.page)), Characters: []Character{}, Links: []Link{}, Fields: []Field{}}
	if result.Width <= 0 || result.Height <= 0 || math.IsNaN(result.Width) || math.IsNaN(result.Height) {
		return result, errors.New("PDF page dimensions are invalid")
	}
	if e.textPage != 0 {
		count := a.charCount(e.textPage)
		if count > 250000 {
			return result, errors.New("PDF page text is too large")
		}
		for i := int32(0); i < count; i++ {
			code := a.unicode(e.textPage, i)
			var left, right, bottom, top float64
			if a.charBox(e.textPage, i, &left, &right, &bottom, &top) == 0 {
				left, right, bottom, top = 0, 0, 0, 0
			}
			result.Characters = append(result.Characters, Character{Text: string(rune(code)), Box: Box{left, result.Height - top, right, result.Height - bottom}})
		}
	}
	var pos int32
	var link uintptr
	for a.enumerateLink(e.page, &pos, &link) != 0 {
		if len(result.Links) >= 10000 {
			break
		}
		var rectangle rect
		if a.linkRect(link, &rectangle) == 0 {
			continue
		}
		value := Link{Box: convertedRect(rectangle, result.Height), Page: -1}
		if dest := a.linkDest(e.document, link); dest != 0 {
			value.Page = int(a.destPage(e.document, dest))
		}
		if action := a.linkAction(link); action != 0 && a.actionType(action) == 3 {
			length := a.uri(e.document, action, nil, 0)
			if length > 0 && length <= 32768 {
				buffer := make([]byte, length)
				a.uri(e.document, action, unsafe.Pointer(&buffer[0]), length)
				value.URL = string(buffer[:len(buffer)-1])
			}
		}
		if value.URL != "" || value.Page >= 0 {
			result.Links = append(result.Links, value)
		}
	}
	if e.form != 0 {
		count := a.annotCount(e.page)
		for i := int32(0); i < count && i < 10000; i++ {
			annot := a.annot(e.page, i)
			if annot == 0 {
				continue
			}
			kind := a.fieldType(e.form, annot)
			if kind > 0 {
				var rectangle rect
				if a.annotRect(annot, &rectangle) != 0 {
					field := Field{Index: int(i), Kind: int(kind), Box: convertedRect(rectangle, result.Height), Name: e.formString(a.fieldName, annot), Value: e.formString(a.fieldValue, annot), ReadOnly: a.fieldFlags(e.form, annot)&1 != 0, Checked: a.checked(e.form, annot) != 0}
					options := a.optionCount(e.form, annot)
					if options > 0 && options < 10000 {
						for option := int32(0); option < options; option++ {
							size := a.optionLabel(e.form, annot, option, nil, 0)
							if size > 0 && size <= 65536 {
								buffer := make([]uint16, (size+1)/2)
								a.optionLabel(e.form, annot, option, unsafe.Pointer(&buffer[0]), size)
								field.Options = append(field.Options, decodeWide(buffer))
							}
						}
					}
					result.Fields = append(result.Fields, field)
				}
			}
			a.closeAnnot(annot)
		}
	}
	return result, nil
}
func decodeWide(buffer []uint16) string {
	for i, n := range buffer {
		if n == 0 {
			buffer = buffer[:i]
			break
		}
	}
	return string(utf16.Decode(buffer))
}
func (e *Engine) formString(get func(uintptr, uintptr, unsafe.Pointer, uint32) uint32, annot uintptr) string {
	size := get(e.form, annot, nil, 0)
	if size == 0 || size > 65536 {
		return ""
	}
	buffer := make([]uint16, (size+1)/2)
	get(e.form, annot, unsafe.Pointer(&buffer[0]), size)
	return decodeWide(buffer)
}
func (e *Engine) Render(index, width, height, rotation int) (*image.RGBA, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ensurePage(index); err != nil {
		return nil, err
	}
	if width < 1 || height < 1 || width > 16000 || height > 16000 || int64(width)*int64(height) > 8*1024*1024 || rotation < 0 || rotation > 3 {
		return nil, errors.New("PDF render dimensions are invalid")
	}
	image := image.NewRGBA(image.Rect(0, 0, width, height))
	var pin runtime.Pinner
	pin.Pin(&image.Pix[0])
	defer pin.Unpin()
	bitmap := e.api.bitmap(int32(width), int32(height), 4, unsafe.Pointer(&image.Pix[0]), int32(image.Stride))
	if bitmap == 0 {
		return nil, errors.New("PDF bitmap allocation failed")
	}
	defer e.api.destroyBitmap(bitmap)
	e.api.fill(bitmap, 0, 0, int32(width), int32(height), 0xffffffff)
	e.api.render(bitmap, e.page, 0, 0, int32(width), int32(height), int32(rotation), 1)
	if e.form != 0 {
		e.api.drawForm(e.form, bitmap, e.page, 0, 0, int32(width), int32(height), int32(rotation), 1)
	}
	// PDFium's external bitmap is BGRA; MyGo's bitmap input is RGBA.
	for i := 0; i < len(image.Pix); i += 4 {
		image.Pix[i], image.Pix[i+2] = image.Pix[i+2], image.Pix[i]
	}
	return image, nil
}
func (e *Engine) SetField(index, annotation int, value string, checked *bool, option *int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ensurePage(index); err != nil {
		return err
	}
	if e.form == 0 {
		return errors.New("PDF form environment unavailable")
	}
	annot := e.api.annot(e.page, int32(annotation))
	if annot == 0 {
		return errors.New("PDF form field unavailable")
	}
	defer e.api.closeAnnot(annot)
	if e.api.fieldFlags(e.form, annot)&1 != 0 {
		return errors.New("PDF form field is read-only")
	}
	kind := e.api.fieldType(e.form, annot)
	defer e.api.killFocus(e.form)
	if kind == FieldCheckbox || kind == FieldRadio {
		if checked == nil {
			return errors.New("PDF checkbox value missing")
		}
		if (e.api.checked(e.form, annot) != 0) == *checked {
			return nil
		}
		var r rect
		if e.api.annotRect(annot, &r) == 0 {
			return errors.New("PDF form rectangle unavailable")
		}
		x, y := float64(r.Left+r.Right)/2, float64(r.Top+r.Bottom)/2
		e.api.clickDown(e.form, e.page, 0, x, y)
		e.api.clickUp(e.form, e.page, 0, x, y)
		return nil
	}
	if e.api.focus(e.form, annot) == 0 {
		return errors.New("PDF form field could not be focused")
	}
	if option != nil && (kind == FieldCombo || kind == FieldList) {
		if e.api.setOption(e.form, e.page, int32(*option), 1) == 0 {
			return errors.New("PDF form option could not be changed")
		}
		return nil
	}
	if kind != FieldText && kind != FieldCombo {
		return errors.New("PDF field is not text-editable")
	}
	e.api.keyDown(e.form, e.page, 65, 2) // FWL_EVENTFLAG_ControlKey, Ctrl+A
	text := append(utf16.Encode([]rune(value)), 0)
	e.api.replace(e.form, e.page, unsafe.Pointer(&text[0]))
	return nil
}
