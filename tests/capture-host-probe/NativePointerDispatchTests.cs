using System.Runtime.InteropServices;
using ShareX.ScreenCaptureLib;

internal static class NativePointerDispatchTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    private static IntPtr At(int x, int y) => new((y << 16) | (x & 0xffff));
    public static int Run()
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private desktop");
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
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
            var saved = 0; Exception? failure = null;
            using var session = new NativeCaptureSession(canvas, _ => {}, _ => saved++);
            var original = canvas.Window.Handle;
            canvas.Window.ShowInTaskbar = true;
            if (canvas.Window.Handle == original) throw new Exception("Handle-recreation test did not recreate the window");
            canvas.SetRegion(new Rectangle(100, 100, 200, 120));
            canvas.Window.Shown += async (_, _) => {
                try {
                    await Task.Delay(100);
                    if (!session.Ready) throw new Exception("A recreated capture handle never became ready");
                    var save = session.Toolbar.Controls.OfType<ToolStrip>().SelectMany(row => row.Items.OfType<ToolStripButton>()).Single(button => button.Name == "save");
                    save.PerformClick();
                    if (saved != 1) throw new Exception("Save stopped working after handle recreation");
                    if (!session.HintVisible) throw new Exception("Successful save had no visible feedback");
                    canvas.AddRectangle(new Rectangle(120, 120, 40, 30));
                    if (session.HintVisible) throw new Exception("Later edits retained the saved-revision message");
                } catch (Exception error) { failure = error; }
                finally { canvas.Window.Close(); }
            };
            canvas.ShowDialog();
            if (failure is not null) System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(failure).Throw();
        }
        Console.WriteLine("PASS: queued down/move/up and down/up drags use their own positions");
        return 0;
    }
}
