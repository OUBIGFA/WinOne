using WinOne.Capture;

internal static class SelectionGeometryTests
{
    public static void Run()
    {
        var bounds = new Rectangle(-200, -100, 400, 200);
        var selected = new Rectangle(-80, -40, 100, 70);
        if (SelectionGeometry.HitEdge(selected, new Point(-80, -40), 6, bounds) != SelectionEdge.TopLeft) throw new Exception("corner handle was not hit");
        if (SelectionGeometry.HitEdge(selected, new Point(-80, 0), 6, bounds) != SelectionEdge.Left) throw new Exception("side handle was not hit");
        if (SelectionGeometry.HitEdge(selected, new Point(0, 0), 6, bounds) != SelectionEdge.None) throw new Exception("interior became an edge");
        if (SelectionGeometry.HitEdge(bounds, new Point(-200, 0), 6, bounds) != SelectionEdge.None) throw new Exception("full desktop capture was resizable");
        if (SelectionGeometry.Resize(selected, SelectionEdge.TopLeft, new Point(-500, -500), bounds) != Rectangle.FromLTRB(-200, -100, 20, 30)) throw new Exception("resize escaped desktop bounds");
        if (SelectionGeometry.Resize(selected, SelectionEdge.BottomRight, new Point(70, 60), bounds) != Rectangle.FromLTRB(-80, -40, 70, 60)) throw new Exception("resize moved opposite edges");
        if (SelectionGeometry.Resize(selected, SelectionEdge.Left, new Point(100, 0), bounds).Width != 1) throw new Exception("resize inverted selection");
        if (SelectionGeometry.FromEdges(new Point(50, 40), new Point(-50, -40), bounds) != new Rectangle(-50, -40, 100, 80))
            throw new Exception("reverse dragging changed the selected area");
        if (SelectionGeometry.FromEdges(new Point(-500, -500), new Point(500, 500), bounds) != bounds)
            throw new Exception("dragging outside the desktop was not constrained");
        if (SelectionGeometry.FromEdges(new Point(0, 0), new Point(0, 0), bounds).Size != Size.Empty)
            throw new Exception("a click became a capture area");
        var viewport = new Rectangle(-1920, 0, 1920, 1080);
        var toolbar = SelectionGeometry.ToolbarBounds(new Rectangle(-300, 950, 250, 100), viewport, new Size(280, 44), 8);
        if (toolbar != new Rectangle(-330, 898, 280, 44)) throw new Exception("toolbar did not move above a bottom-edge selection");
        toolbar = SelectionGeometry.ToolbarBounds(new Rectangle(-1910, 5, 5, 5), viewport, new Size(280, 44), 8);
        if (!viewport.Contains(toolbar)) throw new Exception("toolbar escaped the active monitor");
        var windows = new[] { new Rectangle(-80, -40, 100, 70), new Rectangle(-150, -80, 250, 150) };
        if (SelectionGeometry.WindowAtPoint(windows, new Point(0, 0), bounds) != windows[0]) throw new Exception("window selection ignored stacking order");
        if (SelectionGeometry.WindowAtPoint(windows, new Point(-140, -70), bounds) != windows[1]) throw new Exception("visible part of lower window was not selectable");
        if (!SelectionGeometry.WindowAtPoint(windows, new Point(180, 90), bounds).IsEmpty) throw new Exception("desktop space selected a window");
        var targets = new[] { new WindowCaptureTarget(new IntPtr(1), windows[0]), new WindowCaptureTarget(new IntPtr(2), windows[1]) };
        if (WindowCaptureTargets.ForRegion(targets, new Rectangle(-60, -20, 30, 20))?.Handle != new IntPtr(1)) throw new Exception("scroll region ignored topmost target");
        if (WindowCaptureTargets.ForRegion(targets, new Rectangle(-140, -70, 30, 20))?.Handle != new IntPtr(2)) throw new Exception("visible background region rejected");
        if (WindowCaptureTargets.ForRegion(targets, new Rectangle(-140, -70, 100, 90)) is not null) throw new Exception("partly occluded scroll region accepted");
        if (WindowCaptureTargets.ForRegion(targets, Rectangle.Empty) is not null) throw new Exception("empty scroll region accepted");
    }
}
