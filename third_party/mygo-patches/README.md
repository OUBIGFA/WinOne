# MyGo selected-text adapter

Applies to github.com/egoist/mygo v0.2.13-0.20261006044557-3ad015a7f887 (MIT; the existing result UI notices include its license).

The existing Selectable text editor already stores a selection. One additive method exposes its focused selection to WinOne without modifying the clipboard, layout or Windows accessibility implementation. Password text is excluded. A pointer-release fix commits the final coordinate even when down/up arrive between two frames; its regression test sends both events without an intermediate paint.

build/result-ui-overlay.ps1 copies the pinned source into an ignored build directory, adds selected_text.go.txt as ui/winone_selected_text.go, and generates a temporary module file. build/build-results.ps1 uses that file for tests, vet and compilation. The shared module cache and committed go.mod/go.sum remain unchanged. A dependency version change requires checking this adapter again.
