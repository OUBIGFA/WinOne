using WinOne.Capture.NativeUI;

// Renders the silent-OCR status popup in both themes and languages on a
// private desktop: "recognizing" and "copied" on a grey backdrop. DrawToBitmap
// does not apply the window region, so the rounded corners are drawn by the
// popup itself and the corner pixels show the backdrop only where it paints.
internal static class NativeTextStatusRenderingProbe
{
    internal static int Run(string output)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private test desktop");
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
        Application.EnableVisualStyles();
        var anchor = new Rectangle(200, 200, 600, 120);
        foreach (var dark in new[] { false, true })
        foreach (var english in new[] { false, true }) {
            NativeTextStatus.ShowRecognizing(anchor, dark, english);
            Pump(NativeTextStatus.RecognizingDelayMs + 100);
            Save(output, $"status-{(english ? "en-US" : "zh-CN")}-{(dark ? "dark" : "light")}-recognizing.png", dark);
            NativeTextStatus.ShowCopied();
            Pump(50);
            Save(output, $"status-{(english ? "en-US" : "zh-CN")}-{(dark ? "dark" : "light")}-copied.png", dark);
            Pump(NativeTextStatus.CopiedMs + 200);
            if (NativeTextStatus.Text is not null) throw new Exception("Copied status did not close itself");
        }
        Console.WriteLine("PASS: OCR status popup rendered in both themes and languages and closed itself");
        return 0;
    }

    private static void Pump(int milliseconds)
    {
        var until = Environment.TickCount64 + milliseconds;
        while (Environment.TickCount64 < until) { Application.DoEvents(); Thread.Sleep(10); }
    }

    private static void Save(string output, string name, bool dark)
    {
        var form = Application.OpenForms.Cast<Form>().Single(f => f.Visible);
        if (form.Bounds.Top < 320) throw new Exception("Status popup is not below its selection");
        using var image = new Bitmap(form.Width + 32, form.Height + 32);
        using (var graphics = Graphics.FromImage(image)) graphics.Clear(dark ? Color.FromArgb(60, 60, 60) : Color.FromArgb(200, 200, 200));
        using (var popup = new Bitmap(form.Width, form.Height)) {
            form.DrawToBitmap(popup, new Rectangle(Point.Empty, form.Size));
            using var graphics = Graphics.FromImage(image);
            graphics.DrawImageUnscaled(popup, 16, 16);
        }
        image.Save(Path.Combine(output, name));
    }
}
