package workspace

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

type libraryFixture struct {
	mu            sync.Mutex
	root          string
	payloads      map[string]any
	searchQueries []string
	opened        string
	line          int
}

func librarySetup(t *testing.T) (*libraryFixture, *testhost.Harness, *FileLibrary, *ui.Tester) {
	t.Helper()
	root := t.TempDir()
	f := &libraryFixture{root: root, payloads: map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		value, ok := f.payloads[r.URL.Path]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(value)
	}))
	t.Cleanup(server.Close)
	h := testhost.New(t, func(method string, raw json.RawMessage) (any, error) {
		var input map[string]any
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		if input["id"] != "session" {
			return nil, fmt.Errorf("wrong session")
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		var value any
		switch method {
		case "chat.files.list":
			directory, _ := input["path"].(string)
			if directory == "" {
				directory = root
			}
			if directory == root {
				value = FileListing{Directory: root, Entries: []FileEntry{{Name: "src", Path: filepath.Join(root, "src"), Directory: true}, {Name: "README.md", Path: filepath.Join(root, "README.md")}}}
			} else if directory == filepath.Join(root, "src") {
				value = FileListing{Directory: directory, Entries: []FileEntry{{Name: "main.go", Path: filepath.Join(root, "src", "main.go")}}}
			} else {
				return nil, fmt.Errorf("outside fixture root")
			}
		case "chat.files.index":
			value = FileIndex{Files: []string{"README.md", "src/main.go", "docs/main-guide.md"}}
		case "chat.files.search":
			query, _ := input["query"].(string)
			f.searchQueries = append(f.searchQueries, query)
			if input["searchId"] == "" || input["searchId"] == nil {
				return nil, fmt.Errorf("missing search identity")
			}
			value = SearchResults{Results: []SearchFile{{RelPath: "src/main.go", Matches: []FileMatch{{LineNumber: 12, LineText: query + " proof"}}}}, TotalFiles: 1, TotalMatches: 1}
		case "chat.files.cancel":
			return map[string]bool{"cancelled": true}, nil
		default:
			return nil, fmt.Errorf("unexpected %s", method)
		}
		key := fmt.Sprintf("/result-%d", len(f.payloads))
		f.payloads[key] = value
		return map[string]string{"url": server.URL + key}, nil
	})
	p := NewFileLibrary(h.Env, LibraryOptions{Prefix: "chat.", SessionID: "session", OnOpen: func(path string, line int, _ bool) { f.opened = path; f.line = line }})
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); p.View(c) }, 480, 600)
	for range 3 {
		h.Settle(tt)
	}
	t.Cleanup(p.Close)
	return f, h, p, tt
}
func TestFileTreeLoadsOnlyExpandedDirectoriesAndOpensScopedPath(t *testing.T) {
	f, h, p, tt := librarySetup(t)
	if len(h.Called("chat.files.list")) != 1 || !tt.HasText("README.md") || tt.HasText("main.go") {
		t.Fatalf("unexpected lazy tree: %v", tt.Texts())
	}
	if err := tt.Click("src"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyRight)
	for range 3 {
		h.Settle(tt)
	}
	if len(h.Called("chat.files.list")) != 2 || !tt.HasText("main.go") {
		t.Fatalf("expanding did not load the child directory: calls=%d expanded=%v texts=%v error=%s", len(h.Called("chat.files.list")), p.outline.Open.Has(filepath.Join(f.root, "src")), tt.Texts(), p.err)
	}
	if err := tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	if f.opened != filepath.Join(f.root, "src", "main.go") || p.selected != f.opened {
		t.Fatalf("opened the wrong scoped path: %s", f.opened)
	}
	p.toggle(filepath.Join(f.root, "src"))
	p.toggle(filepath.Join(f.root, "src"))
	for range 3 {
		h.Settle(tt)
	}
	if len(h.Called("chat.files.list")) != 2 {
		t.Fatal("reopening an unchanged folder scanned it again")
	}
}
func TestFilenameFilterKeepsCindysBasenamePriorityAndLimit(t *testing.T) {
	got := filterFiles("MAIN", []string{"main/lib.go", "src/main.go", "main.md", "other.txt", "later-main.txt"}, 3)
	if !reflect.DeepEqual(got, []string{"src/main.go", "main.md", "main/lib.go"}) {
		t.Fatalf("filter order/limit changed: %v", got)
	}
	_, h, p, tt := librarySetup(t)
	if err := tt.Click(wt("filter")); err != nil {
		t.Fatal(err)
	}
	tt.Type("main")
	for range 3 {
		h.Settle(tt)
	}
	if len(h.Called("chat.files.index")) != 1 || len(p.filtered) != 2 {
		t.Fatalf("index/filter unavailable: %s %#v", p.err, p.filtered)
	}
	tt.Type(".go")
	for range 3 {
		h.Settle(tt)
	}
	if len(h.Called("chat.files.index")) != 1 || len(p.filtered) != 1 {
		t.Fatal("every filter keystroke reread the file index")
	}
}
func TestSearchDebouncesAndCarriesItsOwnCancellationIdentity(t *testing.T) {
	f, h, p, tt := librarySetup(t)
	p.mode = "search"
	p.query = "old"
	p.scheduleSearch()
	p.query = "current"
	p.scheduleSearch()
	time.Sleep(300 * time.Millisecond)
	for range 3 {
		h.Settle(tt)
	}
	f.mu.Lock()
	queries := append([]string(nil), f.searchQueries...)
	f.mu.Unlock()
	if !reflect.DeepEqual(queries, []string{"current"}) || p.search.TotalMatches != 1 {
		t.Fatalf("superseded search ran: %v %s", queries, p.err)
	}
	if err := tt.Click("src/main.go"); err != nil {
		t.Fatal(err)
	}
	if f.opened != filepath.Join(f.root, "src", "main.go") || f.line != 12 {
		t.Fatal("search result did not preserve its line target")
	}
	p.searchID = "cancel-this-search"
	p.cancelSearch()
	for range 3 {
		h.Settle(tt)
	}
	cancelled := h.Called("chat.files.cancel")
	if len(cancelled) != 1 || !strings.Contains(string(cancelled[0].Params), "cancel-this-search") {
		t.Fatalf("search cancellation lost its identity: %#v", cancelled)
	}
}
func TestHiddenTreeDoesNotReactToWindowFocus(t *testing.T) {
	_, h, p, tt := librarySetup(t)
	focus := 0
	h.Env.Focuses = func() int { return focus }
	p.SetVisible(false)
	focus++
	for range 3 {
		h.Settle(tt)
	}
	if len(h.Called("chat.files.list")) != 1 {
		t.Fatal("hidden tree read on focus")
	}
}
