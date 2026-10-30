// Package icons holds the window's icons: Lucide's (ISC), generated from
// the locked lucide-react package into lucide/ by
// build/generate-mygo-icons.mjs, and the few drawn elsewhere, in custom/.
//
// Use an icon by name, as icons.Save; run the generator after referencing
// a Lucide icon the window has not used yet.
package icons

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

//go:embed lucide custom
var files embed.FS

func lucide(name string) *ui.SVG { return parse("lucide/" + name + ".svg") }

func custom(name string) *ui.SVG { return parse("custom/" + name + ".svg") }

func parse(path string) *ui.SVG {
	data, err := files.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}
