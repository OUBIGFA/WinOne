using System.Runtime.InteropServices;
using ShareX.ScreenCaptureLib;

internal static class NativeViewportSessionTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    public static void Run()
    {
        using var image = new Bitmap(800, 600);
        using (var graphics = Graphics.FromImage(image)) graphics.Clear(Color.LightBlue);
        using var canvas = new WinOneCaptureCanvas(image, viewport: true);
        canvas.SetRegion(new Rectangle(50, 50, 700, 500));
        var copies = 0; Size outputSize = default;
        using var session = new NativeCaptureSession(canvas, output => { copies++; outputSize = output.Size; }, _ => throw new Exception("Unexpected save"));
        using var timeout = new System.Windows.Forms.Timer { Interval = 10000 };
        var started = false; Exception? failure = null;
        timeout.Tick += (_, _) => { failure = new TimeoutException("Viewport session timed out"); canvas.Window.Close(); };
        session.Updated += () => {
            if (started || !session.Ready) return;
            started = true;
            canvas.Window.BeginInvoke((Action)(() => {
                try {
                    canvas.SetViewport(0.5f, new PointF(100, 100));
                    var text = new NativeTextInputState(null, "WinOne 缩放输入", 100, 150, 500, 80, 400, 24, Color.Red.ToArgb(), 2, 3);
                    session.RestoreText(text);
                    var input = session.TextInput ?? throw new Exception("Missing inline input");
                    var handle = input.Handle;
                    foreach (var (scale, origin) in new[] { (1.5f, new PointF(-30, 15)), (0.75f, new PointF(55, 25)), (0.5f, new PointF(100, 100)) }) {
                        canvas.SetViewport(scale, origin);
                        var expected = canvas.ImageToView(text.Bounds);
                        if (session.Error is not null) throw new Exception(session.Error);
                        if (input.Handle != handle || input.Text != text.Value || input.Font.Size != text.FontPixels * scale ||
                            input.Left != expected.Left || input.Top != expected.Top || input.Width != expected.Width ||
                            input.SelectionStart != 2 || input.SelectionLength != 3)
                            throw new Exception("Zoom changed the inline input identity, content, caret or placement");
                        var snapshot = session.Snapshot() ?? throw new Exception("Missing viewport snapshot");
                        if (snapshot.Text?.X != text.X || snapshot.Text.Y != text.Y || snapshot.Text.Width != text.Width || snapshot.Text.FontPixels != 24)
                            throw new Exception("Viewport geometry leaked into persistent text coordinates");
                        if (!canvas.Window.ClientRectangle.Contains(session.Toolbar.Bounds)) throw new Exception("Viewport toolbar escaped the window");
                    }
                    NativeScrollInteractionTests.Confirm(session);
                } catch (Exception error) { failure = error; canvas.Window.Close(); }
            }));
        };
        timeout.Start(); canvas.ShowDialog(); timeout.Stop();
        if (failure is not null) throw failure;
        var layer = canvas.SnapshotDocument()!.Layers.Single(layer => layer.Kind == "text");
        if (copies != 1 || outputSize != new Size(700, 500) || layer.FontPixels != 24 || layer.Bounds.X != 100 || layer.Bounds.Y != 150 || layer.Bounds.Width != 500)
            throw new Exception("Zoomed text confirmation changed full-resolution output");
        Console.WriteLine("PASS: zoom preserves one inline HWND, caret, image coordinates, bounded toolbar and original output size");
    }
}
