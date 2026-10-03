using System.Reflection;
using ShareX.ScreenCaptureLib;

internal static class NativePresentationTests
{
    private const BindingFlags Members = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
    private static object Get(object target, string name) => target.GetType().GetProperty(name, Members)!.GetValue(target)!;
    private static void Set(object target, string name, object value) => target.GetType().GetProperty(name, Members)!.SetValue(target, value);
    private static void Mouse(RegionCaptureForm form, Point point, MouseButtons previous, MouseButtons current)
    {
        var input = Get(form, "InputManager");
        object State(MouseButtons buttons) {
            object state = new MouseState();
            Set(state, "Buttons", buttons); Set(state, "ClientPosition", point); Set(state, "Position", point);
            return state;
        }
        input.GetType().GetField("oldMouseState", Members)!.SetValue(input, State(previous));
        input.GetType().GetField("mouseState", Members)!.SetValue(input, State(current));
    }
    public static void Run()
    {
        CursorStates();
        ResizeCursorStates();
        // Drive the engine's resize path without moving the user's pointer.
        foreach (var edge in new[] { "Left", "Top", "Right", "Bottom" }) {
            using var source = new Bitmap(800, 600);
            using var canvas = new WinOneCaptureCanvas(source);
            var form = (RegionCaptureForm)canvas.Window;
            var manager = Get(form, "ShapeManager");
            canvas.SetRegion(new Rectangle(100, 100, 400, 300));
            var target = form.RectangleToScreen(new Rectangle(80, 80, 440, 340));
            canvas.UpdateWindowCandidates(new[] { target });
            var shape = Get(manager, "CurrentShape");
            shape.GetType().GetMethod("OnNodePositionUpdate", Members)!.Invoke(shape, null);
            var nodes = (Array)Get(manager, "ResizeNodes");
            var node = nodes.GetValue((int)Enum.Parse(typeof(RegionCaptureForm).Assembly.GetType("ShareX.ScreenCaptureLib.NodePosition")!, edge))!;
            var start = Point.Round((PointF)Get(node, "Position"));
            Set(manager, "NodesVisible", true);
            Set(node, "IsDragging", true);
            Mouse(form, start, MouseButtons.None, MouseButtons.Left);
            var update = manager.GetType().GetMethod("UpdateNodes", Members)!;
            update.Invoke(manager, null);
            var near = edge switch {
                "Left" => new Point(83, start.Y), "Top" => new Point(start.X, 83),
                "Right" => new Point(516, start.Y), _ => new Point(start.X, 416)
            };
            Mouse(form, near, MouseButtons.Left, MouseButtons.Left);
            update.Invoke(manager, null);
            var expected = edge switch {
                "Left" => Rectangle.FromLTRB(80, 100, 500, 400),
                "Top" => Rectangle.FromLTRB(100, 80, 500, 400),
                "Right" => Rectangle.FromLTRB(100, 100, 520, 400),
                _ => Rectangle.FromLTRB(100, 100, 500, 420)
            };
            if (canvas.Selection != expected) throw new Exception($"{edge} did not snap: {canvas.Selection}, expected {expected}");
            // Moving away must release the edge; no sticky offset or opposite-edge drift.
            Mouse(form, start, MouseButtons.Left, MouseButtons.Left); update.Invoke(manager, null);
            if (canvas.Selection != new Rectangle(100, 100, 400, 300)) throw new Exception($"{edge} did not release snap");
        }
        Console.WriteLine("PASS: native four-edge resize snaps and releases without changing the opposite edge");
        Preview();
    }

    private static void CursorStates()
    {
        using var source = new Bitmap(800, 600);
        using var canvas = new WinOneCaptureCanvas(source);
        var form = (RegionCaptureForm)canvas.Window;
        if (form.Cursor != Cursors.Default) throw new Exception("Idle capture must show the normal pointer");
        canvas.BeginRegion(new Rectangle(100, 100, 1, 1)); form.SetDefaultCursor();
        if (form.Cursor != Cursors.Default) throw new Exception("A click without dragging must keep the normal pointer");
        var manager = Get(form, "ShapeManager");
        Set(Get(manager, "CurrentShape"), "Rectangle", new RectangleF(100, 100, 80, 60));
        form.SetDefaultCursor();
        if (form.Cursor != Cursors.Cross) throw new Exception("Dragging a selection must show the crosshair");
        canvas.FinishGesture(); form.SetDefaultCursor();
        if (form.Cursor != Cursors.Default) throw new Exception("Completed selection must restore the normal pointer");
        Console.WriteLine("PASS: capture uses a normal pointer except during selection dragging");
    }

    private static void ResizeCursorStates()
    {
        using var image = new Bitmap(800, 600);
        using var canvas = new WinOneCaptureCanvas(image);
        var form = (RegionCaptureForm)canvas.Window;
        canvas.SetRegion(new Rectangle(100, 100, 400, 300));
        var manager = Get(form, "ShapeManager");
        Set(manager, "NodesVisible", true);
        var nodes = (Array)Get(manager, "ResizeNodes");
        var positions = typeof(RegionCaptureForm).Assembly.GetType("ShareX.ScreenCaptureLib.NodePosition")!;
        foreach (var (name, cursor) in new[] {
            ("Left", Cursors.SizeWE), ("Right", Cursors.SizeWE), ("Top", Cursors.SizeNS), ("Bottom", Cursors.SizeNS),
            ("TopLeft", Cursors.SizeNWSE), ("BottomRight", Cursors.SizeNWSE), ("TopRight", Cursors.SizeNESW), ("BottomLeft", Cursors.SizeNESW)
        }) {
            var node = nodes.GetValue((int)Enum.Parse(positions, name))!;
            Set(node, "Visible", true); Set(node, "IsCursorHover", true); form.SetHandCursor(false);
            if (form.Cursor != cursor) throw new Exception(name + " hover must show the matching resize pointer");
            Set(node, "IsDragging", true); Set(node, "IsCursorHover", false); form.SetHandCursor(true);
            if (form.Cursor != cursor) throw new Exception(name + " drag lost its resize pointer outside the handle");
            Set(node, "IsDragging", false); form.SetDefaultCursor();
            if (form.Cursor != Cursors.Default) throw new Exception(name + " release did not restore the normal pointer");
        }
        Console.WriteLine("PASS: all resize handles use directional pointers during hover and drag");
    }

    private static void Preview()
    {
        using var source = new Bitmap(800, 600);
        using (var g = Graphics.FromImage(source)) g.Clear(Color.White);
        using var canvas = new WinOneCaptureCanvas(source);
        var form = (RegionCaptureForm)canvas.Window;
        Set(form, "ClientArea", new Rectangle(0, 0, 800, 600));
        var manager = Get(form, "ShapeManager");
        var candidate = new RectangleRegionShape { Rectangle = new RectangleF(100, 100, 300, 200) };
        Set(candidate, "Manager", manager);
        Set(manager, "CurrentHoverShape", candidate);
        using var pixels = new Bitmap(800, 600);
        using (var g = Graphics.FromImage(pixels)) {
            g.Clear(Color.Gray);
            g.SetClip(new Rectangle(0, 0, 250, 600));
            form.GetType().GetMethod("DrawWinOneCandidate", Members)!.Invoke(form, new object[] { g });
        }
        if (pixels.GetPixel(150, 150).ToArgb() != Color.White.ToArgb() ||
            pixels.GetPixel(300, 150).ToArgb() != Color.Gray.ToArgb() ||
            pixels.GetPixel(50, 50).ToArgb() != Color.Gray.ToArgb())
            throw new Exception("Candidate preview did not preserve original pixels and paint clipping");
        Console.WriteLine("PASS: unconfirmed selection shows original pixels and preserves the paint clip");
    }
}
