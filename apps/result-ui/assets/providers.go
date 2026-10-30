// Package assets shares the result window's artwork of AI providers and
// model families with the desktop window, so both show the same icons from
// one inventory. The result window embeds these files itself; this package
// only exposes them to other modules.
package assets

import "embed"

// Providers holds the brand marks (providers/*.svg, and providers/*.png for
// marks that only exist as pictures) and their 128 px samples
// (provider-bitmaps/*.png), which build/build-provider-icons.ps1 renders
// with resvg before any window is built.
//
//go:embed providers/*.svg providers/*.png provider-bitmaps/*.png
var Providers embed.FS
