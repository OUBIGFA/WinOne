//go:build windows && nativee2e

package main

// Test builds only. The production build's mygo_noinspector tag also keeps
// this socket out; never ship the native automation endpoint in the installer.
import _ "github.com/egoist/mygo/plugins/automation"
