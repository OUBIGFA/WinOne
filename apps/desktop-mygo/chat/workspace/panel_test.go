package workspace

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
)

func panelTester(p *Panel) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); p.View(c) }, 1000, 650)
}
func TestNativePanelKeepsDirtyDocumentAcrossTabChoiceReorderAndReopen(t *testing.T) {
	_, h, doc, _ := textSetup(t, "notes.txt")
	p := NewPanel(h.Env, PanelOptions{Prefix: "chat.", SessionID: "session"})
	t.Cleanup(p.Close)
	p.tabs = []PanelTab{{ID: "one", Kind: "file", Path: "notes.txt", Title: "One"}, {ID: "two", Kind: "file", Path: "notes.txt", Title: "Two"}}
	p.selected, p.section = "one", "tabs"
	p.bodies["one"] = &DocumentPane{env: h.Env, text: doc, body: doc, preview: doc.Metadata(), options: DocumentOptions{Prefix: "chat.", SessionID: "session", Path: "notes.txt"}}
	// A second view may point to the same file, but does not own the first buffer.
	p.bodies["two"] = &DocumentPane{env: h.Env, preview: FilePreview{Name: "Two", Kind: "binary"}, options: DocumentOptions{Path: "notes.txt"}}
	doc.Buffer().Set("Keep this draft")
	tt := panelTester(p)
	h.Settle(tt)
	if !p.Dirty() {
		t.Fatal("panel did not see the native buffer draft")
	}
	p.reorder("two", "one")
	if p.tabs[0].ID != "two" || !p.Dirty() || doc.Buffer().String() != "Keep this draft" {
		t.Fatal("reordering reset the document")
	}
	p.requestClose("one")
	if p.guard == nil || len(p.tabs) != 2 {
		t.Fatal("dirty tab closed without an explicit choice")
	}
	p.guard = nil
	p.SetVisible(false)
	p.SetVisible(true)
	if doc.Buffer().String() != "Keep this draft" {
		t.Fatal("hiding destroyed a draft")
	}
	p.requestClose("one")
	p.saveGuard()
	for range 3 {
		h.Settle(tt)
	}
	if p.guard != nil || len(p.tabs) != 1 || p.closedTabs[0].ID != "one" {
		t.Fatal("confirmed native save did not close the tab")
	}
	if p.Dirty() {
		t.Fatal("closed saved document left the panel dirty")
	}
}
func TestChoosingSameLibraryFileAtALineKeepsUnsavedText(t *testing.T) {
	_, h, doc, _ := textSetup(t, "notes.txt")
	p := NewPanel(h.Env, PanelOptions{Prefix: "chat.", SessionID: "session"})
	t.Cleanup(p.Close)
	p.libraryPath = "notes.txt"
	p.libraryDocument = &DocumentPane{env: h.Env, text: doc, body: doc, preview: doc.Metadata()}
	doc.Buffer().Set("Unsaved text\nsecond line")
	p.chooseLibraryFile("notes.txt", 2, false)
	if p.libraryDocument.text != doc || doc.Buffer().String() != "Unsaved text\nsecond line" || !doc.Dirty() {
		t.Fatal("line navigation replaced the open buffer")
	}
	p.chooseLibraryFile("other.txt", 7, false)
	if p.guard == nil || p.guard.switchTo != "other.txt" || p.guard.switchLine != 7 {
		t.Fatal("switch confirmation lost the destination line")
	}
}
func TestPanelCloseSequenceStopsAtTheFirstDirtyTab(t *testing.T) {
	_, h, doc, _ := textSetup(t, "notes.txt")
	p := NewPanel(h.Env, PanelOptions{Prefix: "chat.", SessionID: "session"})
	t.Cleanup(p.Close)
	p.tabs = []PanelTab{{ID: "clean", Kind: "file"}, {ID: "dirty", Kind: "file"}, {ID: "after", Kind: "file"}}
	p.bodies["dirty"] = &DocumentPane{env: h.Env, text: doc, body: doc}
	doc.Buffer().Set("Unsaved")
	p.closeMany([]string{"clean", "dirty", "after"})
	if len(p.tabs) != 2 || p.guard == nil || p.guard.id != "dirty" || len(p.closeQueue) != 1 || p.closeQueue[0] != "after" {
		t.Fatal("bulk tab closing passed a dirty document")
	}
	p.guard = nil
	p.closeQueue = nil
	if doc.Buffer().String() != "Unsaved" {
		t.Fatal("cancelling the close sequence lost its draft")
	}
}
