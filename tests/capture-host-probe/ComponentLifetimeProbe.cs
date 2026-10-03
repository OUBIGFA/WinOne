using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text.Json;
using ShareX.ScreenCaptureLib;

internal static class ComponentLifetimeProbe
{
    [DllImport("user32.dll")] private static extern uint GetGuiResources(IntPtr process, uint kind);
    public static int Run(string mode, string sourcePath, string output)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private desktop");
        if (mode is not ("canvas" or "canvas-viewport" or "inline" or "inline-static" or "toolbar" or "pin-raw" or "pin-managed")) throw new ArgumentException("Unknown component");
        if (Directory.Exists(output)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        using var source = new Bitmap(sourcePath);
        using var process = Process.GetCurrentProcess();
        var observations = new List<object>();
        var lateGdi = new List<uint>();
        for (var i = 0; i < 40; i++) {
            if (mode is "canvas" or "canvas-viewport") {
                using var capture = new WinOneCaptureCanvas(source, viewport: mode == "canvas-viewport");
                capture.SetRegion(new Rectangle(100, 100, 700, 400));
                capture.Window.Shown += (_, _) => { capture.Window.Refresh(); capture.Window.BeginInvoke(capture.Window.Close); };
                capture.ShowDialog();
            } else if (mode is "inline" or "inline-static") {
                using var window = new Form { ClientSize = new Size(800, 600), ShowInTaskbar = false };
                using var input = new NativeInlineText(window, source, new Rectangle(100, 100, 400, 120), "WinOne 文字\n连续输入");
                if (mode == "inline") { input.SetTextStyle(18, Color.Red); input.SetTextStyle(24, Color.Red); }
                window.Shown += (_, _) => { window.Refresh(); window.BeginInvoke(window.Close); };
                window.ShowDialog();
            } else if (mode == "toolbar") {
                using var window = new Form { ClientSize = new Size(800, 140), ShowInTaskbar = false };
                using var toolbar = new NativeToolbarControl();
                window.Controls.Add(toolbar); toolbar.Size = toolbar.PreferredToolbarSize;
                window.Shown += (_, _) => { window.Refresh(); window.BeginInvoke(window.Close); };
                window.ShowDialog();
            } else if (mode == "pin-managed") {
                ShareX.PinToScreenForm.OpenManaged(new Bitmap(700, 400), new ShareX.PinToScreenOptions { Shadow = false });
                Application.DoEvents(); ShareX.PinToScreenForm.CloseAll();
            } else {
                var constructor = typeof(ShareX.PinToScreenForm).GetConstructor(System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.NonPublic,
                    null, new[] { typeof(Image), typeof(ShareX.PinToScreenOptions), typeof(Point?) }, null)!;
                using var window = (Form)constructor.Invoke(new object?[] { new Bitmap(700, 400), new ShareX.PinToScreenOptions { Shadow = false }, null });
                window.Show(); window.Refresh(); window.Close();
            }
            process.Refresh();
            observations.Add(new { iteration = i + 1, gdi = GetGuiResources(process.Handle, 0), user = GetGuiResources(process.Handle, 1), privateBytes = process.PrivateMemorySize64 });
            if (i >= 20) lateGdi.Add(GetGuiResources(process.Handle, 0));
        }
        File.WriteAllText(Path.Combine(output, "components.json"), JsonSerializer.Serialize(new { mode, observations }));
        if (mode is "inline" or "inline-static" && lateGdi.Max() - lateGdi.Min() > 2)
            throw new Exception("Closed inline inputs keep accumulating GDI handles after warmup");
        return 0;
    }
}
