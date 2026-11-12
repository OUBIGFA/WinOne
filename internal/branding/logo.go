package branding

import _ "embed"

// Logo is the full-size master artwork, as the desktop window's title bar
// shows it. Programs that do not reference it do not link it.
//
//go:embed logo.png
var Logo []byte
