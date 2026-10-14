using System.Runtime.InteropServices;
using ShareX.ScreenCaptureLib;

internal static class NativePointerDispatchTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    private static IntPtr At(int x, int y) => new((y << 16) | (x & 0xffff));
    private static bool Same(WinOneCaptureLayer a, WinOneCaptureLayer b) => System.Text.Json.JsonSerializer.Serialize(a) == System.Text.Json.JsonSerializer.Serialize(b);
    private static byte[] RenderBytes(WinOneCaptureCanvas canvas) {
        using var bitmap = canvas.RenderSelection();
        using var stream = new MemoryStream();
        bitmap.Save(stream, System.Drawing.Imaging.ImageFormat.Png);
        return stream.ToArray();
    }
    public static int Run()
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private desktop");
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        foreach (var tool in new[] { "rectangle", "ellipse", "arrow", "pen", "cover", "mosaic", "blur", "text" }) {
            using var image = new Bitmap(800, 600);
            // Effects need nonuniform pixels: a flat image could conceal a
            // broken mosaic/blur relocation while geometry assertions pass.
            using (var graphics = Graphics.FromImage(image)) {
                graphics.Clear(Color.White);
                for (var x = 0; x < image.Width; x += 7) graphics.FillRectangle(Brushes.DarkBlue, x, 0, 3, image.Height);
            }
            using var canvas = new WinOneCaptureCanvas(image);
            canvas.SetRegion(new Rectangle(20, 20, 700, 500));
            if (tool == "text") canvas.AddText("Move text", new Rectangle(150, 150, 100, 60), 24);
            else canvas.AddDrawing(tool, new Rectangle(150, 150, 100, 60), new[] { new PointF(150, 150), new PointF(200, 180), new PointF(250, 210) });
            canvas.AddRectangle(new Rectangle(400, 300, 80, 60));
            canvas.SelectTool("select");
            var before = canvas.ExportDocument();
            var beforePixels = RenderBytes(canvas);
            Exception? failure = null;
            canvas.Window.Shown += (_, _) => canvas.Window.BeginInvoke((Action)(() => {
                try {
                    // Real window messages, isolated from the user's pointer.
                    var start = tool == "ellipse" ? new Point(200, 150) : new Point(150, 150);
                    SendMessageW(canvas.Window.Handle, 0x0200, IntPtr.Zero, At(start.X, start.Y));
                    SendMessageW(canvas.Window.Handle, 0x0201, new IntPtr(1), At(start.X, start.Y));
                    SendMessageW(canvas.Window.Handle, 0x0200, new IntPtr(1), At(start.X + 35, start.Y + 25));
                    SendMessageW(canvas.Window.Handle, 0x0202, IntPtr.Zero, At(start.X + 35, start.Y + 25));
                    var after = canvas.ExportDocument();
                    if (after.Selection != before.Selection || after.Layers.Length != before.Layers.Length) throw new Exception(tool + " move changed selection or layer count");
                    if (after.Layers[0].Bounds.X != before.Layers[0].Bounds.X + 35 || after.Layers[0].Bounds.Y != before.Layers[0].Bounds.Y + 25) throw new Exception(tool + " did not move by the pointer delta");
                    if (!Same(after.Layers[1], before.Layers[1])) throw new Exception(tool + " move changed another layer");
                    var afterPixels = RenderBytes(canvas);
                    if (beforePixels.SequenceEqual(afterPixels)) throw new Exception(tool + " move did not change output pixels");
                    canvas.Undo();
                    if (!Same(canvas.ExportDocument().Layers[0], before.Layers[0])) throw new Exception(tool + " undo failed");
                    if (!RenderBytes(canvas).SequenceEqual(beforePixels)) throw new Exception(tool + " undo did not restore output pixels");
                    canvas.Redo();
                    if (!Same(canvas.ExportDocument().Layers[0], after.Layers[0])) throw new Exception(tool + " redo failed");
                    if (!RenderBytes(canvas).SequenceEqual(afterPixels)) throw new Exception(tool + " redo did not restore moved output pixels");
                } catch (Exception error) { failure = error; }
                finally { canvas.Window.Close(); }
            }));
            canvas.ShowDialog();
            if (failure is not null) System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(failure).Throw();
        }
        foreach (var moveMessage in new[] { true, false }) {
            using var image = new Bitmap(800, 600);
            using var canvas = new WinOneCaptureCanvas(image);
            Exception? failure = null;
            canvas.Window.Shown += (_, _) => canvas.Window.BeginInvoke((Action)(() => {
                try {
                    // Replay queued pointer messages without changing global
                    // cursor position/buttons. Each message owns its coordinates.
                    SendMessageW(canvas.Window.Handle, 0x0201, new IntPtr(1), At(100, 100));
                    if (moveMessage) SendMessageW(canvas.Window.Handle, 0x0200, new IntPtr(1), At(180, 160));
                    SendMessageW(canvas.Window.Handle, 0x0202, IntPtr.Zero, At(180, 160));
                    if (canvas.Selection != new Rectangle(100, 100, 81, 61))
                        throw new Exception($"Queued drag lost its event positions (move={moveMessage}): {canvas.Selection}");
                } catch (Exception error) { failure = error; }
                finally { canvas.Window.Close(); }
            }));
            canvas.ShowDialog();
            if (failure is not null) System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(failure).Throw();
        }
        using (var image = new Bitmap(800, 600))
        using (var canvas = new WinOneCaptureCanvas(image)) {
            var saved = 0; var copied = 0; var closed = 0; var scheduled = false; var timedOut = false; Exception? failure = null;
            using var session = new NativeCaptureSession(canvas, _ => copied++, bitmap => {
                if (bitmap.Size != new Size(200, 120)) throw new Exception("Save changed the selection after handle recreation");
                saved++;
            });
            var original = canvas.Window.Handle;
            canvas.Window.ShowInTaskbar = true;
            if (canvas.Window.Handle == original) throw new Exception("Handle-recreation test did not recreate the window");
            canvas.SetRegion(new Rectangle(100, 100, 200, 120));
            canvas.Window.FormClosed += (_, _) => closed++;
            using var deadline = new System.Windows.Forms.Timer { Interval = 4000 };
            deadline.Tick += (_, _) => { timedOut = true; canvas.Window.Close(); };
            session.Updated += () => {
                if (!session.Ready || scheduled) return;
                scheduled = true;
                canvas.Window.BeginInvoke((Action)(() => {
                    try {
                        var save = session.Toolbar.Controls.OfType<ToolStrip>().SelectMany(row => row.Items.OfType<ToolStripButton>()).Single(button => button.Name == "save");
                        save.PerformClick();
                    } catch (Exception error) { failure = error; canvas.Window.Close(); }
                }));
            };
            // A modal form finishes closing when its message loop returns;
            // the button callback must not close the window on the test's behalf.
            deadline.Start(); canvas.ShowDialog(); deadline.Stop();
            if (failure is not null) System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(failure).Throw();
            if (timedOut || saved != 1 || closed != 1 || !session.Saved || !session.CloseRequested || canvas.Window.Visible)
                throw new Exception("Successful save did not exit after handle recreation");
            if (copied != 0 || session.Copied) throw new Exception("Saving also copied the capture");
        }
        Console.WriteLine("PASS: eight annotation types move individually with undo/redo; selection and other layers preserved; queued drag positions and handle recreation");
        return 0;
    }
}
