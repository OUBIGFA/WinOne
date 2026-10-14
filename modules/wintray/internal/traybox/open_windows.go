//go:build windows

package traybox

import "context"

// OpenIcon combines the upstream registration check before a delayed double
// click with WinOne's process-creation and before/after window checks.
// Call it off the UI thread; latest dispatches only its current-icon lookup.
func OpenIcon(ctx context.Context, icon Icon, latest func() (Icon, error)) error {
	return openAutomatically(ctx, icon, latest)
}
