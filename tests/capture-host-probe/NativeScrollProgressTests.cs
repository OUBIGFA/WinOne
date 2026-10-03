using System.Runtime.InteropServices;
using System.Text.Json;

internal static class NativeScrollProgressTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    [DllImport("user32.dll")] private static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll", EntryPoint = "GetWindowLongPtrW")] private static extern IntPtr GetWindowLongPtr(IntPtr window, int index);
    public static int Run(string output)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private test desktop");
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        var cases = new List<object>();
        foreach (var dark in new[] { false, true }) foreach (var language in new[] { "zh-CN", "en-US" }) {
            var requests = 0;
            using var window = new NativeScrollProgress(() => requests++, dark, language);
            var before = GetForegroundWindow();
            window.Show();
            var initialSize = window.Size;
            if (GetForegroundWindow() != before) throw new Exception("Progress window changed foreground focus");
            if ((GetWindowLongPtr(window.Handle, -20).ToInt64() & 0x08000000) == 0 ||
                SendMessageW(window.Handle, 0x0021, IntPtr.Zero, IntPtr.Zero) != new IntPtr(3))
                throw new Exception("Progress window lost its no-activation contract");
            using (var image = new Bitmap(window.Width, window.Height)) {
                window.DrawToBitmap(image, new Rectangle(Point.Empty, image.Size));
                image.Save(Path.Combine(output, $"progress-{language}-{dark}.png"));
            }
            var button = window.Controls.Find("scroll-stop", true).OfType<Button>().Single();
            var label = window.Controls.OfType<FlowLayoutPanel>().Single().Controls.OfType<Label>().Single();
            var labelSize = label.Size; var buttonBounds = button.Bounds;
            if (!window.ClientRectangle.Contains(window.RectangleToClient(button.RectangleToScreen(button.ClientRectangle))))
                throw new Exception("Stop button is clipped");
            button.PerformClick();
            SendMessageW(window.Handle, 0x0312, new IntPtr(0x574f), IntPtr.Zero);
            if (requests != 1 || button.Enabled) throw new Exception("Stop was not idempotent");
            if (window.Size != initialSize) throw new Exception($"Stopping resized panel {initialSize} -> {window.Size}, label {labelSize} -> {label.Size}, button {buttonBounds} -> {button.Bounds}");
            cases.Add(new { dark, language, window.Width, window.Height, requests });
        }
        File.WriteAllText(Path.Combine(output, "verification.json"), JsonSerializer.Serialize(new {
            cases, noActivation = true, screenshotExclusionVerified = false, globalHotkeyVerified = false,
            scope = "Private window rendering and message dispatch; no physical input or real capture"
        }));
        Console.WriteLine("PASS: native scrolling stop control, no-activation styles and bilingual themes");
        return 0;
    }
}
