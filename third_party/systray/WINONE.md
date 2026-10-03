# WinOne adaptation

Source: `fyne.io/systray v1.12.2`. License: Apache-2.0, retained in `LICENSE`.

Runtime sources are included; examples and upstream tests are excluded. WinOne's
Windows regression tests cover its changes separately.

WinOne adds an in-memory PNG menu-icon API for changing tray images without
temporary icon files or a cache entry for every animation frame. Replaced and
removed menu bitmaps are released. The root module uses a local `replace` so the
global Go module cache remains unchanged.

WinOne also corrects the Windows NOTIFYICONDATAW timeout/version union layout
and adds a one-shot warning notification API. Notification text is bounded
without splitting UTF-16 pairs. Windows quiet-time preferences are respected;
later icon or tooltip updates do not replay an earlier notification.
Clicking a notification uses the same application-open callback as the tray icon.
