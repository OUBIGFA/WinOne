using System.Text.Json;
using ShareX.ScreenCaptureLib;

internal static class NativePaintDispatchProbe
{
    private sealed class Counter : NativeWindow, IDisposable
    {
        public int Paints { get; private set; }
        public Counter(Control control) => AssignHandle(control.Handle);
        protected override void WndProc(ref Message message) { if (message.Msg == 0x000f) Paints++; base.WndProc(ref message); }
        public void Dispose() => ReleaseHandle();
    }
    public static int Run(string output)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private test desktop");
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        using var image = new Bitmap(ShareX.HelpersLib.CaptureHelpers.GetScreenBounds().Width, ShareX.HelpersLib.CaptureHelpers.GetScreenBounds().Height);
        using var canvas = new WinOneCaptureCanvas(image);
        using var session = new NativeCaptureSession(canvas, _ => {}, _ => {});
        using var parent = new Counter(canvas.Window);
        var counters = session.Toolbar.Controls.Cast<Control>().ToDictionary(control => control.Name, control => new Counter(control));
        var observations = new List<object>();
        canvas.Window.Shown += async (_, _) => {
            canvas.SetRegion(new Rectangle(100, 100, 700, 400));
            // Observe the real message pump without DrawToBitmap, PrintWindow,
            // Refresh or mouse hover forcing child controls to repaint.
            await Task.Delay(400);
            if (counters.Values.Any(counter => counter.Paints == 0))
                throw new Exception("A visible toolbar row never received its first paint");
            observations.Add(new { stage = "first-show", parent = parent.Paints, children = counters.ToDictionary(pair => pair.Key, pair => pair.Value.Paints) });
            session.Toolbar.Location = new Point(200, 600);
            await Task.Delay(400);
            observations.Add(new { stage = "moved", parent = parent.Paints, children = counters.ToDictionary(pair => pair.Key, pair => pair.Value.Paints) });
            canvas.Window.Close();
        };
        try { canvas.ShowDialog(); }
        finally { foreach (var counter in counters.Values) counter.Dispose(); }
        File.WriteAllText(Path.Combine(output, "paint-dispatch.json"), JsonSerializer.Serialize(observations));
        Console.WriteLine(JsonSerializer.Serialize(observations));
        return 0;
    }
}
