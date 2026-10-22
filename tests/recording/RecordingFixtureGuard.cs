using System.Runtime.InteropServices;

// The live test may capture only its own opaque target. Abort if another
// window covers it; never treat real desktop content as a generated fixture.
internal static class RecordingFixtureGuard
{
    [DllImport("user32.dll")] private static extern IntPtr GetTopWindow(IntPtr parent);
    [DllImport("user32.dll")] private static extern IntPtr GetWindow(IntPtr window, uint command);
    [DllImport("user32.dll")] private static extern bool IsWindowVisible(IntPtr window);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr window, out uint processId);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] private static extern int GetClassName(IntPtr window, System.Text.StringBuilder name, int count);
    [DllImport("user32.dll")] private static extern bool GetWindowRect(IntPtr window, out Rect bounds);
    [DllImport("user32.dll")] private static extern bool GetWindowDisplayAffinity(IntPtr window, out uint affinity);
    [DllImport("user32.dll", SetLastError = true)] private static extern bool SetWindowPos(IntPtr window, IntPtr after, int x, int y, int width, int height, uint flags);
    [DllImport("dwmapi.dll")] private static extern int DwmFlush();
    [DllImport("dwmapi.dll")] private static extern int DwmGetWindowAttribute(IntPtr window, int attribute, out int value, int size);
    [StructLayout(LayoutKind.Sequential)] private struct Rect { public int Left, Top, Right, Bottom; }
    internal static void Arrange(Form fixture, Rectangle region, params Form[] overlays)
    {
        // Shown runs before WinForms finishes its initial activation. Establish
        // the test-only Z order after all HWNDs exist, without touching another
        // application's topmost state or relying on a guessed startup delay.
        foreach (var window in new[] { fixture }.Concat(overlays)) {
            if (!SetWindowPos(window.Handle, new IntPtr(-1), 0, 0, 0, 0, 0x0013))
                throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error(), "Could not show the owned recording fixture above its backdrop");
        }
        Marshal.ThrowExceptionForHR(DwmFlush());
        Verify(fixture, region, overlays);
    }
    internal static async Task Wait(Form fixture, Rectangle region, int milliseconds, params Form[] overlays)
    {
        // Keep the safety check active during pause/static-tail waits as well,
        // not just when the fixture happens to repaint a new animation frame.
        var clock = System.Diagnostics.Stopwatch.StartNew();
        while (clock.ElapsedMilliseconds < milliseconds) {
            Verify(fixture, region, overlays);
            var remaining = milliseconds - clock.ElapsedMilliseconds;
            if (remaining > 0) await Task.Delay((int)Math.Min(30, remaining));
        }
        Verify(fixture, region, overlays);
    }
    internal static void Verify(Form fixture, Rectangle region, params Form[] overlays)
    {
        if (!fixture.Visible || !GetWindowDisplayAffinity(fixture.Handle, out var affinity) || affinity != 0)
            throw new InvalidOperationException("The owned recording fixture is hidden or excluded from capture");
        var allowed = overlays.Where(window => !window.IsDisposed).Select(window => window.Handle).ToHashSet();
        var visited = new HashSet<IntPtr>();
        for (var window = GetTopWindow(IntPtr.Zero); window != IntPtr.Zero; window = GetWindow(window, 2)) {
            if (!visited.Add(window) || visited.Count > 4096) throw new InvalidOperationException("Window order changed during the owned recording test");
            if (window == fixture.Handle) return;
            if (!IsWindowVisible(window) || allowed.Contains(window)) continue;
            if (DwmGetWindowAttribute(window, 14, out var cloaked, sizeof(int)) == 0 && cloaked != 0) continue;
            if (!GetWindowRect(window, out var bounds)) throw new InvalidOperationException("Could not verify recording fixture visibility");
            if (Rectangle.FromLTRB(bounds.Left, bounds.Top, bounds.Right, bounds.Bottom).IntersectsWith(region)) {
                GetWindowThreadProcessId(window, out var owner);
                var name = new System.Text.StringBuilder(128); GetClassName(window, name, name.Capacity);
                // The real native tooltip is intentionally hovered over the
                // fixture. Permit only our own already-excluded tooltip HWND.
                if (owner == Environment.ProcessId && WinOne.Capture.NativeUI.RecordingWindowProtection.IsToolTipWindow(window) && GetWindowDisplayAffinity(window, out var tooltipAffinity) && tooltipAffinity == 0x11) continue;
                throw new InvalidOperationException($"The owned recording fixture is obscured: class={name}, pid={owner}, handle={window}, bounds={bounds.Left},{bounds.Top},{bounds.Right},{bounds.Bottom}, fixture={fixture.Handle}; stop capture");
            }
        }
        throw new InvalidOperationException("Owned recording fixture disappeared from native window order");
    }
}
