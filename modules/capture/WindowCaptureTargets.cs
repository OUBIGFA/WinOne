using System.Runtime.InteropServices;
using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture;

internal record WindowCaptureTarget(IntPtr Handle, Rectangle Bounds, Rectangle? ClientBounds = null);

internal static class WindowCaptureTargets
{
    // The first intersecting window owns the visible pixels. Do not scroll a
    // lower window when an overlay covers even part of the selected region.
    internal static WindowCaptureTarget? ForRegion(IReadOnlyList<WindowCaptureTarget> windows, Rectangle region)
    {
        if (region.Width <= 0 || region.Height <= 0) return null;
        foreach (var window in windows)
        {
            if (!window.Bounds.IntersectsWith(region)) continue;
            return window.Bounds.Contains(region) ? window : null;
        }
        return null;
    }

    [DllImport("user32.dll")]
    private static extern IntPtr GetTopWindow(IntPtr parent);
    [DllImport("user32.dll")]
    private static extern IntPtr GetWindow(IntPtr window, uint command);
    [DllImport("user32.dll")]
    private static extern uint GetDpiForWindow(IntPtr window);

    internal static int EdgeWidth(WindowCaptureTarget target)
    {
        var dpi = GetDpiForWindow(target.Handle);
        return (int)Math.Max(1, Math.Round(12 * (dpi == 0 ? 96 : dpi) / 96.0));
    }

    // Like ShareX WindowsRectangleList, select descendants before their parent
    // and include the client area separately from the outer DWM frame. Keep
    // each top-level window's regions together to preserve desktop Z order.
    internal static IReadOnlyList<Rectangle> Regions(IReadOnlyList<WindowCaptureTarget> windows)
    {
        var result = new List<Rectangle>();
        var visited = new HashSet<IntPtr>();
        var rectangles = new HashSet<Rectangle>();
        var inspected = 0;
        void Add(Rectangle rectangle)
        {
            if (rectangle.Width > 0 && rectangle.Height > 0 && rectangles.Add(rectangle)) result.Add(rectangle);
        }
        void Visit(IntPtr handle, Rectangle bounds, int depth)
        {
            if (!visited.Add(handle)) throw new InvalidOperationException("窗口区域正在变化，请重试");
            if (visited.Count > 16384 || depth > 64) throw new InvalidOperationException("窗口区域数量超过处理上限");
            var client = Rectangle.Intersect(bounds, NativeMethods.GetClientRect(handle));
            if (!client.IsEmpty)
            {
                var siblings = new HashSet<IntPtr>();
                for (var child = GetTopWindow(handle); child != IntPtr.Zero; child = GetWindow(child, 2))
                {
                    if (!siblings.Add(child)) throw new InvalidOperationException("窗口区域正在变化，请重试");
                    if (++inspected > 16384) throw new InvalidOperationException("窗口区域数量超过处理上限");
                    if (!NativeMethods.IsWindowVisible(child)) continue;
                    var childBounds = Rectangle.Intersect(client, NativeMethods.GetWindowRect(child));
                    if (childBounds.Width > 0 && childBounds.Height > 0) Visit(child, childBounds, depth + 1);
                }
                if (depth == 0) Add(client);
            }
            Add(bounds);
        }
        foreach (var window in windows) Visit(window.Handle, window.Bounds, 0);
        return result;
    }

    // Walk explicit Z order, bounded against handles changing during traversal.
    // Filtering and DWM frame bounds follow ShareX's WindowsRectangleList.
    internal static IReadOnlyList<WindowCaptureTarget> Snapshot()
    {
        var visited = new HashSet<IntPtr>();
        var result = new List<WindowCaptureTarget>();
        for (var handle = GetTopWindow(IntPtr.Zero); handle != IntPtr.Zero; handle = GetWindow(handle, 2))
        {
            if (!visited.Add(handle)) throw new InvalidOperationException("窗口列表正在变化，请重试");
            if (visited.Count > 4096) throw new InvalidOperationException("窗口数量超过处理上限");
            var info = new WindowInfo(handle);
            if (!info.IsVisible || info.IsMinimized || info.IsCloaked) continue;
            if (info.ClassName is "Progman" or "WorkerW" or "CEF-OSC-WIDGET") continue;
            if (info.ExStyle.HasFlag(WindowStyles.WS_EX_TOOLWINDOW | WindowStyles.WS_EX_NOACTIVATE)) continue;
            var bounds = CaptureHelpers.GetWindowRectangle(handle);
            if (bounds.Width > 0 && bounds.Height > 0) result.Add(new WindowCaptureTarget(handle, bounds, Rectangle.Intersect(bounds, NativeMethods.GetClientRect(handle))));
        }
        return result;
    }
}
