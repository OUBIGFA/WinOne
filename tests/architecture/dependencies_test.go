// Package architecture checks the dependency directions that let independent
// feature changes stay local. Parse all platform imports, without compiling UI
// or requiring the private upstream resource checkouts on the CI machine.
package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestProductionDependencyDirections(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	for _, directory := range []string{"internal", "apps/host", "apps/desktop-mygo", "apps/result-ui", "modules/nativeui"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				switch entry.Name() {
				case "_temp", "dist", "node_modules", ".git", "bin", "obj":
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			for _, imported := range parsed.Imports {
				dependency, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					return err
				}
				if reason := forbiddenDependency(relative, dependency); reason != "" {
					t.Errorf("%s imports %s: %s", relative, dependency, reason)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func forbiddenDependency(file, dependency string) string {
	ui := strings.HasPrefix(dependency, "winone/desktopmygo") || strings.HasPrefix(dependency, "winone/resultui") ||
		strings.HasPrefix(dependency, "winone/modules/nativeui") || strings.HasPrefix(dependency, "github.com/egoist/mygo")
	if (strings.HasPrefix(file, "internal/") || strings.HasPrefix(file, "apps/host/")) && ui {
		return "backend and shared services must not depend on a UI implementation"
	}
	if strings.HasPrefix(file, "modules/nativeui/") && strings.HasPrefix(dependency, "winone/") && !strings.HasPrefix(dependency, "winone/modules/nativeui/") {
		return "shared UI components must not depend on a feature or application"
	}
	if page, ok := strings.CutPrefix(file, "apps/desktop-mygo/pages/"); ok {
		if target, ok := strings.CutPrefix(dependency, "winone/desktopmygo/pages/"); ok {
			owner, _, _ := strings.Cut(page, "/")
			other, _, _ := strings.Cut(target, "/")
			if owner != other && strings.Contains(page, "/") {
				return "feature pages communicate through shared contracts, not sibling page implementations"
			}
		}
	}
	return ""
}

func TestDependencyRulesAllowContractsAndRejectReverseEdges(t *testing.T) {
	for _, sample := range []struct {
		file, dependency string
		forbidden        bool
	}{
		{"internal/ai/runtime.go", "winone/desktopmygo/core", true},
		{"apps/host/main.go", "winone/resultui", true},
		{"modules/nativeui/modelmenu/menu.go", "winone/internal/ai", true},
		{"apps/desktop-mygo/pages/capture/capture.go", "winone/desktopmygo/pages/aiservices", true},
		{"apps/desktop-mygo/pages/pages.go", "winone/desktopmygo/pages/aiservices", false},
		{"apps/desktop-mygo/pages/capture/capture.go", "winone/internal/captureprefs", false},
		{"modules/nativeui/modelmenu/menu.go", "winone/modules/nativeui/design", false},
	} {
		if got := forbiddenDependency(sample.file, sample.dependency) != ""; got != sample.forbidden {
			t.Errorf("%s -> %s: forbidden=%v", sample.file, sample.dependency, got)
		}
	}
}
