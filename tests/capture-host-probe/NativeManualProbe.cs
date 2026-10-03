using WinOne.Capture;

// Interactive visual acceptance uses the real workflow and screen acquisition.
// A taskbar entry makes the otherwise tool-only form discoverable by Windows
// automation; it does not force painting or alter the canvas/toolbar controls.
internal static class NativeManualProbe
{
    public static int Run(string directory)
    {
        directory = Path.GetFullPath(directory); Directory.CreateDirectory(directory);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        var settings = new SettingsStore(directory);
        settings.Save(settings.Snapshot() with { OpenEditor = true, CopyToClipboard = false, OutputDirectory = Path.Combine(directory, "images") });
        var operations = new CaptureOperations(settings, directory);
        try {
            NativeCaptureWorkflow.Run(settings, operations, "window", "zh-CN", CancellationToken.None,
                show: (canvas, session) => {
                    canvas.Window.ShowInTaskbar = true;
                    canvas.Window.Text = "WinOne 截图实操";
                    canvas.ShowDialog();
                }, drafts: new NativeCaptureDraftStore(directory));
        } catch (OperationCanceledException error) { File.WriteAllText(Path.Combine(directory, "cancelled.txt"), error.ToString()); }
        catch (Exception error) { File.WriteAllText(Path.Combine(directory, "error.txt"), error.ToString()); return 1; }
        return 0;
    }
}
