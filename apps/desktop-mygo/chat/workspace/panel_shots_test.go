package workspace

import (
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

// panelShotHost answers what the panel's sections ask of one conversation:
// its files, a text file, its resources and a turn's review.
func panelShotHost(t *testing.T) *testhost.Harness {
	t.Helper()
	root := `E:\WinOne`
	source := "# 发布说明\n\n本次更新整理了 Chat 的阅读排版。\n\n- 回复正文使用 15 号字\n- 代码块带复制按钮\n\n```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\n"
	payloads := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/text":
			fmt.Fprint(w, source)
		case r.URL.Path == "/patch":
			fmt.Fprint(w, reviewPatch)
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payloads[r.URL.Path])
		}
	}))
	t.Cleanup(server.Close)
	return testhost.New(t, func(method string, raw json.RawMessage) (any, error) {
		var input map[string]any
		_ = json.Unmarshal(raw, &input)
		var value any
		switch method {
		case "chat.files.list":
			directory, _ := input["path"].(string)
			if directory == "" || directory == root {
				value = FileListing{Directory: root, Entries: []FileEntry{
					{Name: "apps", Path: filepath.Join(root, "apps"), Directory: true},
					{Name: "docs", Path: filepath.Join(root, "docs"), Directory: true},
					{Name: "README.md", Path: filepath.Join(root, "README.md")},
					{Name: "notes.md", Path: filepath.Join(root, "notes.md")},
					{Name: "a-file-with-a-very-long-name-that-needs-an-ellipsis.config.json", Path: filepath.Join(root, "long.json")}}}
			} else {
				value = FileListing{Directory: directory, Entries: []FileEntry{{Name: "development.md", Path: filepath.Join(directory, "development.md")}}}
			}
		case "chat.files.index":
			value = FileIndex{Files: []string{"README.md", "notes.md", "docs/development.md"}}
		case "chat.file.preview":
			path, _ := input["path"].(string)
			return FilePreview{Path: path, Name: filepath.Base(path), Kind: "text", MIME: "text/markdown", URL: server.URL + "/text", Size: int64(len(source)), Revision: "r1"}, nil
		case "chat.resources":
			return json.RawMessage(`{"agents":[{"path":"E:\\WinOne\\AGENTS.md"}],"skills":[{"name":"visual-design","description":"创建或评审界面的排版、布局、颜色与交互视觉时使用。","path":"E:\\skills\\visual-design\\SKILL.md"},{"name":"code-rules","description":"修改、调试或审查代码时，遵循通用的改动范围与验证约定。","path":"E:\\skills\\code-rules\\SKILL.md"}],"plugins":[{"name":"GitHub","path":"E:\\plugins\\github","enabled":true}],"mcpServers":[{"name":"Documents","enabled":true,"status":"connected"}],"diagnostics":[]}`), nil
		case "chat.review.snapshot":
			return ReviewSnapshot{Root: root, Branch: "main", Scope: "working", Revision: "snapshot-one", Files: []ReviewFile{{Path: "new.go", OldPath: "old.go", Status: "R", Additions: 3, Deletions: 3}, {Path: "docs/development.md", Status: "M", Additions: 12, Deletions: 1}}}, nil
		case "chat.review.metadata":
			return ReviewMetadata{Repository: true, Root: root, Branch: "main", HeadOID: strings.Repeat("b", 40)}, nil
		case "chat.review.commits":
			return []ReviewCommit{}, nil
		case "chat.review.diff.preview":
			return map[string]string{"url": server.URL + "/patch"}, nil
		default:
			return nil, fmt.Errorf("unexpected %s", method)
		}
		key := fmt.Sprintf("/result-%d", len(payloads))
		payloads[key] = value
		return map[string]string{"url": server.URL + key}, nil
	})
}

// TestPanelShots draws the side panel's sections in both themes and two
// widths when WINONE_SHOTS names a folder, for looking at.
func TestPanelShots(t *testing.T) {
	dir := os.Getenv("WINONE_SHOTS")
	if dir == "" {
		t.Skip("WINONE_SHOTS is not set")
	}
	i18n.SetLocale(i18n.Chinese)
	for _, dark := range []bool{false, true} {
		for _, width := range []int{460, 760} {
			h := panelShotHost(t)
			p := NewPanel(h.Env, PanelOptions{Prefix: "chat.", SessionID: "session", Directory: `E:\WinOne`})
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(kit.Theme(c.Preferences(), dark))
				// As the Chat page hosts it: on the canvas, beside a hairline.
				ui.Column(c).Fill().Background(c.Theme().Background).Children(func() { p.View(c) })
			}, width, 760)
			shot := func(name string) {
				t.Helper()
				for range 4 {
					h.Settle(tt)
				}
				file, err := os.Create(filepath.Join(dir, fmt.Sprintf("panel-%s-%s-%d.png", name, map[bool]string{false: "light", true: "dark"}[dark], width)))
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				if err := png.Encode(file, tt.Image()); err != nil {
					t.Fatal(err)
				}
			}
			shot("files")
			p.chooseLibraryFile(`E:\WinOne\notes.md`, 0, false)
			shot("files-open")
			p.OpenLibrary("resources", "")
			shot("resources")
			p.section = "tabs"
			shot("tabs-empty")
			p.OpenFile(`E:\WinOne\notes.md`, 0)
			p.OpenFile(`E:\WinOne\docs\development.md`, 0)
			p.OpenReview("turn", "")
			shot("tabs-review")
			p.selected = p.tabs[0].ID
			shot("tabs-file")
			p.Close()
		}
	}
}
