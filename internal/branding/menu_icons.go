package branding

import _ "embed"

// Official Lucide AppWindow and LogOut, rasterized from the locked frontend
// package by build/generate-capture-icons.mjs --tray-only. Menus tint the alpha
// template using their own theme and DPI; source license is kept beside it.
//
//go:embed menu-icons/open.png
var TrayOpenIcon []byte

//go:embed menu-icons/exit.png
var TrayExitIcon []byte
