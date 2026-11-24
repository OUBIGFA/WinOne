package branding

import _ "embed"

// Solar Linear window-frame and logout-2, rasterized from the locked Solar
// source by build/generate-capture-icons.mjs --tray-only. Menus tint the alpha
// template using their own theme and DPI; source license is kept beside it.
//
//go:embed menu-icons/open.png
var TrayOpenIcon []byte

//go:embed menu-icons/exit.png
var TrayExitIcon []byte
