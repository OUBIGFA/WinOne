// Package branding contains generated application artwork.
package branding

import _ "embed"

//go:embed icon.ico
var Icon []byte

// NativeLogo is a small derivative for native menus; keep the full-size master
// out of GDI icon allocations. The common build regenerates it with the ICO.
//
//go:embed logo-native.png
var NativeLogo []byte
