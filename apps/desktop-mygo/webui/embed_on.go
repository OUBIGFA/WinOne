//go:build webui

package webui

import (
	"embed"
	"io/fs"
)

// dist is chat-embed.html and what it loads, as `npm run build:mygo` in
// apps/desktop/frontend builds it (build/build-desktop-mygo.ps1 runs it
// and builds with this tag).
//
//go:embed all:dist
var dist embed.FS

func init() { files, _ = fs.Sub(dist, "dist") }
