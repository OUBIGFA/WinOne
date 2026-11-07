package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// A visible translation key is a broken product label. Check literal calls
// across the native application, including options hidden behind expanders.
// Dynamic expressions are covered by each page's state and render tests.
func TestNativeLiteralTranslationsExist(t *testing.T) {
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "T" {
				return true
			}
			owner, ok := selector.X.(*ast.Ident)
			if !ok || owner.Name != "i18n" {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			key, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			for _, locale := range []string{Chinese, English} {
				if Translate(locale, key, "count", 2) == key {
					t.Errorf("%s: missing %s %s", path, locale, key)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
