package convenience

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
	"winone/internal/directorylinks"
)

func linkRecord(id, target, status string) directorylinks.Record {
	return directorylinks.Record{ID: id, Name: id, LinkPath: `C:\links\` + id, TargetPath: target, Kind: "hardlink", ObjectType: "file",
		TargetText: target, CreatedAt: "2026-10-16T10:00:00Z", CheckedAt: "2026-10-16T11:00:00Z", Status: status, Reason: "same-file",
		Accessible: true, EntryIdentity: directorylinks.Identity{File: "known"}}
}

func TestLinkPathsCompareAsObjects(t *testing.T) {
	cases := map[string]string{`C:/Docs/./a/../B\`: `c:\docs\b`, `\\server\share\x`: `\\server\share\x`, `c:`: `c:`}
	for path, key := range cases {
		if got := pathKey(path); got != key {
			t.Errorf("pathKey(%q) = %q, want %q", path, got, key)
		}
	}
	if got := pathsFromText("\"C:\\one.txt\"\n\nc:/ONE.txt\nC:\\two"); !slices.Equal(got, []string{"c:/ONE.txt", `C:\two`}) {
		t.Fatalf("paths %q", got)
	}
}

func TestLinkGroupsPutFoldersFirstAndNumbersInOrder(t *testing.T) {
	folder := linkRecord("d", `C:\folder`, "active")
	folder.ObjectType = "directory"
	rows := []directorylinks.Record{linkRecord("a", "C:/docs/part10.txt", "active"), linkRecord("b", `c:\docs\part2.txt`, "active"),
		linkRecord("c", `C:\DOCS\part2.txt`, "active"), folder}
	var got [][]string
	for _, group := range groupLinks(rows) {
		var ids []string
		for _, r := range group.records {
			ids = append(ids, r.ID)
		}
		got = append(got, ids)
	}
	want := [][]string{{"d"}, {"b", "c"}, {"a"}}
	if len(got) != len(want) {
		t.Fatalf("groups %v", got)
	}
	for i := range want {
		if !slices.Equal(got[i], want[i]) {
			t.Fatalf("groups %v", got)
		}
	}
}

// linkHost answers the link service; records is the history, created
// what createBatch returns.
func linkHost(t *testing.T, records []directorylinks.Record, handle func(method string, params json.RawMessage) (any, bool)) *testhost.Harness {
	return testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		if value, ok := handle(method, params); ok {
			return value, nil
		}
		switch method {
		case "directory-links.pending":
			return map[string]any{"requested": false, "paths": []string{}}, nil
		case "directory-links.menu.get":
			return directorylinks.ExplorerMenuState{}, nil
		case "directory-links.list", "directory-links.inspectBatch":
			return records, nil
		case "directory-links.browse":
			var args struct {
				Directory string `json:"directory"`
			}
			_ = json.Unmarshal(params, &args)
			return directorylinks.DirectoryListing{Directory: args.Directory, Parent: `C:\`, Entries: []directorylinks.BrowseEntry{}}, nil
		}
		t.Errorf("unexpected %s %s", method, params)
		return nil, nil
	})
}

func linksView(tab *linksTab) func(*ui.Context) {
	return func(c *ui.Context) { kit.Page(c, func() { tab.View(c, false) }) }
}

func TestLinkHistoryForgetsOnlyInvalidRecords(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	active, broken, offline := linkRecord("valid", `C:\valid.txt`, "active"), linkRecord("invalid", `C:\invalid.txt`, "broken"), linkRecord("offline", `C:\offline.txt`, "unavailable")
	var called []string
	h := linkHost(t, []directorylinks.Record{active, broken, offline}, func(method string, params json.RawMessage) (any, bool) {
		called = append(called, method)
		if method == "directory-links.forgetInvalid" {
			return map[string]any{"records": []directorylinks.Record{active, offline}, "forgotten": 1}, true
		}
		return nil, false
	})
	tab := newLinksTab(h.Env)
	tt := ui.NewTester(linksView(tab), 1100, 1600)
	h.Settle(tt)
	if tt.HasText("一键删除失效历史") {
		t.Fatal("history shows outside the find tab")
	}
	if err := tt.Click("查找已有链接"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if !tt.HasText("invalid.txt") || tt.HasText(`C:\links\invalid`) {
		t.Fatalf("groups start closed %q", tt.Texts())
	}
	if err := tt.Click("一键删除失效历史"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if !tt.HasText("已删除 1 条失效历史") {
		t.Fatalf("notice %q", tt.Texts())
	}
	if len(tab.records) != 2 || slices.ContainsFunc(tab.records, func(r directorylinks.Record) bool { return r.ID == "invalid" }) {
		t.Fatalf("records %v", tab.records)
	}
	if slices.Contains(called, "directory-links.remove") || slices.Contains(called, "directory-links.forget") {
		t.Fatalf("calls %v", called)
	}
}

func TestLinkRemovalAsksAndSendsTheCheckedRecord(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	row := linkRecord("r1", `C:\source.txt`, "active")
	var removed json.RawMessage
	h := linkHost(t, []directorylinks.Record{row}, func(method string, params json.RawMessage) (any, bool) {
		if method == "directory-links.remove" {
			removed = params
			next := row
			next.Status, next.Reason = "removed", "recycled"
			return next, true
		}
		return nil, false
	})
	tab := newLinksTab(h.Env)
	tab.tab = linksScan
	tt := ui.NewTester(linksView(tab), 1100, 1800)
	h.Settle(tt)
	if err := tt.Click("source.txt"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if err := tt.Click("移除链接入口"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if removed != nil || !tt.HasText(row.LinkPath) {
		t.Fatalf("removed without asking: %s", removed)
	}
	if err := tt.Click("移到回收站"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	var args map[string]any
	if err := json.Unmarshal(removed, &args); err != nil || args["id"] != "r1" || args["confirmed"] != true || args["checkedAt"] != row.CheckedAt {
		t.Fatalf("remove %s", removed)
	}
	if tab.records[0].Status != "removed" {
		t.Fatalf("record %+v", tab.records[0])
	}
}

func TestLinkPrefillCreatesWithOneStep(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	created := directorylinks.Record{ID: "one", Name: "one", LinkPath: `C:\links\one.txt`, TargetPath: `C:\one.txt`, Kind: "hardlink", ObjectType: "file", Status: "active"}
	var previewed json.RawMessage
	h := linkHost(t, nil, func(method string, params json.RawMessage) (any, bool) {
		switch method {
		case "directory-links.previewBatch":
			previewed = params
			return directorylinks.BatchPlan{ID: "batch-1", Items: []directorylinks.Plan{{ID: "p1", Source: `C:\one.txt`, LinkPath: `C:\links\one.txt`, Kind: "hardlink", ObjectType: "file"}}}, true
		case "directory-links.createBatch":
			return directorylinks.BatchResult{Records: []directorylinks.Record{created}, Completed: 1, Total: 1}, true
		}
		return nil, false
	})
	tab := newLinksTab(h.Env)
	tab.Queue(coreIntent([]string{`C:\one.txt`}))
	tt := ui.NewTester(linksView(tab), 1200, 1600)
	h.Settle(tt)
	if !tt.HasText("one.txt") {
		t.Fatalf("prefill not shown %q", tt.Texts())
	}
	if err := tt.Click("链接存放文件夹"); err != nil {
		t.Fatal(err)
	}
	tt.Type(`C:\links`)
	h.Settle(tt)
	if err := tt.Click("创建链接"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	var args struct {
		Sources   []string `json:"sources"`
		Directory string   `json:"directory"`
		Kind      string   `json:"kind"`
	}
	if err := json.Unmarshal(previewed, &args); err != nil || !slices.Equal(args.Sources, []string{`C:\one.txt`}) || args.Directory != `C:\links` || args.Kind != "auto" {
		t.Fatalf("preview %s", previewed)
	}
	if !tt.HasText("已核查生效 1 / 1 项") {
		t.Fatalf("result %q", tt.Texts())
	}
	if tab.batch.text != "" || len(tab.records) != 1 {
		t.Fatalf("after creating: sources %q records %d", tab.batch.text, len(tab.records))
	}
}

func coreIntent(paths []string) core.Intent { return core.Intent{Requested: true, Paths: paths} }
