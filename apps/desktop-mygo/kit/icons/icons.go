// Package icons holds the window's icons: Solar Linear (MIT, artwork CC BY
// 4.0 by 480 Design), generated into solar/ by build/generate-mygo-icons.mjs
// from the locked source recorded in third_party/solar-icons.json.
//
// Use an icon by name, as icons.Save; map a new name in
// third_party/solar-icons.json and run the generator.
package icons

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

//go:embed solar/*.svg
var files embed.FS

func solar(name string) *ui.SVG {
	data, err := files.ReadFile("solar/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}
