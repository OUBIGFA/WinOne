package workspace

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/pdfium"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

func TestPDFTextGeometrySelectionAndUnicodeSearch(t *testing.T) {
	p := &PDFPane{page: pdfium.Page{Width: 100, Height: 200, Characters: []pdfium.Character{
		{Text: "A", Box: pdfium.Box{Left: 10, Top: 20, Right: 20, Bottom: 30}},
		{Text: "中", Box: pdfium.Box{Left: 20, Top: 20, Right: 30, Bottom: 30}},
		{Text: "B", Box: pdfium.Box{Left: 10, Top: 40, Right: 20, Bottom: 50}},
	}}, anchor: 0, selectionEnd: 1}
	if p.SelectedText() != "A中" {
		t.Fatal("native PDF selection changed its text")
	}
	matches := findPDFText(p.page, "中b")
	if len(matches) != 1 || matches[0].start != 1 || matches[0].end != 2 {
		t.Fatalf("unicode PDF search lost glyph identities: %#v", matches)
	}
	boxes := p.selectionRects(ui.Rect{X: 5, Y: 10, W: 200, H: 400})
	if len(boxes) != 1 || boxes[0].X != 25 || boxes[0].Y != 50 || boxes[0].W != 40 {
		t.Fatalf("selection coalescing changed geometry: %#v", boxes)
	}
	for rotation, expected := range [][2]float64{{10, 20}, {180, 10}, {90, 180}, {20, 90}} {
		x, y := pdfPoint(10, 20, 100, 200, rotation)
		if x != expected[0] || y != expected[1] {
			t.Fatalf("rotation %d: %f,%f", rotation, x, y)
		}
	}
}
func pdfPaneSetup(t *testing.T) (*testhost.Harness, *PDFPane, *ui.Tester) {
	t.Helper()
	worker := os.Getenv("WINONE_PDF_WORKER")
	if worker == "" {
		t.Skip("explicit verified PDF helper is required")
	}
	path := filepath.Join("..", "..", "..", "..", "tests", "native-pdf", "fixtures", "forms-and-links.pdf")
	// Tests execute in apps/desktop-mygo/chat/workspace, four levels below root.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)
	h := testhost.New(t, func(method string, raw json.RawMessage) (any, error) {
		var input map[string]string
		_ = json.Unmarshal(raw, &input)
		if method != "chat.file.preview" || input["id"] != "owned" || input["path"] != "forms.pdf" {
			return nil, fmt.Errorf("wrong document scope")
		}
		return FilePreview{Path: "forms.pdf", Name: "forms.pdf", Kind: "pdf", URL: server.URL, Size: int64(len(data))}, nil
	})
	p := NewPDFPane(h.Env, PDFOptions{Prefix: "chat.", SessionID: "owned", Path: "forms.pdf", Worker: worker})
	t.Cleanup(p.Close)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); p.View(c) }, 700, 750)
	for range 5 {
		h.Settle(tt)
	}
	return h, p, tt
}
func TestRealNativePDFPaneRendersSelectsFindsAndFillsForms(t *testing.T) {
	h, p, tt := pdfPaneSetup(t)
	if p.count != 1 || p.bitmap == nil || p.err != "" || p.page.Width != 300 {
		t.Fatalf("PDF native pane failed: %s count=%d", p.err, p.count)
	}
	p.anchor, p.selectionEnd = 0, 4
	if p.SelectedText() != "Hello" {
		t.Fatalf("text selection=%q", p.SelectedText())
	}
	p.query = "PDF"
	p.find()
	for range 4 {
		h.Settle(tt)
	}
	if len(p.matches) != 1 || !strings.Contains(p.SelectedText(), "PDF") {
		t.Fatalf("document find failed: %#v %q", p.matches, p.SelectedText())
	}
	p.openForms()
	for _, draft := range p.forms {
		if draft.field.Kind == pdfium.FieldText {
			draft.value = "Filled natively"
		}
	}
	p.applyForms()
	for range 5 {
		h.Settle(tt)
	}
	if p.err != "" {
		t.Fatal(p.err)
	}
	filled := false
	for _, field := range p.page.Fields {
		if field.Value == "Filled natively" {
			filled = true
		}
	}
	if !filled {
		t.Fatal("native form input did not reach PDFium")
	}
}
