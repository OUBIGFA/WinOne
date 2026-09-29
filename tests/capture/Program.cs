using ShareX.ScreenCaptureLib;

internal static class Program
{
    // This opt-in probe captures only its own solid-colour window, never the user's desktop.
    [STAThread]
    private static int Main(string[] args)
    {
        if (args.Length != 1) return 2;
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
        var result = 1;
        using var form = new Form
        {
            Text = "WinOne capture verification",
            ClientSize = new Size(160, 120),
            BackColor = Color.FromArgb(80, 110, 140),
            FormBorderStyle = FormBorderStyle.FixedToolWindow,
            TopMost = true,
            StartPosition = FormStartPosition.CenterScreen
        };
        form.Shown += async (_, _) =>
        {
            try
            {
                // The harness starts processes hidden; explicitly reveal this test-owned fixture.
                form.Hide();
                form.Show();
                form.Activate();
                await Task.Delay(300);
                var origin = form.PointToScreen(new Point(20, 20));
                using var capture = new Screenshot().CaptureRectangle(new Rectangle(origin, new Size(32, 32)));
                if (capture == null || capture.Width != 32 || capture.Height != 32)
                    throw new InvalidOperationException("Incorrect captured dimensions");
                var pixel = capture.GetPixel(16, 16);
                if (pixel.ToArgb() != form.BackColor.ToArgb())
                    throw new InvalidOperationException($"Wrong captured pixel: {pixel}");
                File.WriteAllText(args[0], "PASS: ShareX captured the fixture at 32x32 with the expected pixel");
                result = 0;
            }
            catch (Exception error)
            {
                File.WriteAllText(args[0], "FAIL: " + error);
            }
            finally { form.Close(); }
        };
        Application.Run(form);
        return result;
    }
}
