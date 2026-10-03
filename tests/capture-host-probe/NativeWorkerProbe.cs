using System.Diagnostics;
using System.Text.Json;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;

internal static class NativeWorkerProbe
{
    public static int Run(string imagePath, string assets, string output, string pipe)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private test desktop");
        output = Path.GetFullPath(output);
        if (Directory.Exists(output)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        Application.SetUnhandledExceptionMode(UnhandledExceptionMode.ThrowException);
        Environment.SetEnvironmentVariable("WINONE_PROBE_NATIVE_TOOLBAR", "1");
        var data = Path.Combine(output, "data");
        var settings = new SettingsStore(data);
        settings.Save(settings.Snapshot() with { OutputDirectory = Path.Combine(output, "images"), CopyToClipboard = false, Theme = Environment.GetEnvironmentVariable("WINONE_PROBE_THEME") ?? "system" });
        SettingsStore.AtomicWrite(Path.Combine(data, "managed-settings.json"), JsonSerializer.SerializeToUtf8Bytes(settings.Snapshot(), SettingsStore.Json));
        var sequence = 0; var copies = 0;
        using var worker = new CaptureApplication(pipe, data, (store, operations, mode, language, cancellation) => {
            var clock = Stopwatch.StartNew();
            var session = Path.Combine(output, "session-" + ++sequence); Directory.CreateDirectory(session);
            try {
            return NativeCaptureWorkflow.Run(store, operations, mode, language, cancellation,
                acquire: () => {
                    var source = new Bitmap(imagePath);
                    try { return CapturedFrame.Own(source, ShareX.HelpersLib.CaptureHelpers.GetScreenBounds()); }
                    catch { source.Dispose(); throw; }
                }, windows: () => Array.Empty<WindowCaptureTarget>(), show: (capture, nativeSession) => {
                    using var regions = Environment.GetEnvironmentVariable("WINONE_PROBE_TEST_WINDOW_REGIONS") == "1"
                        ? new NativeWindowRegions(capture, new[] { new WindowCaptureTarget(new IntPtr(1), new Rectangle(capture.Window.Left + 100, capture.Window.Top + 100, 700, 400)) },
                            (_, _) => Task.FromResult<IReadOnlyList<Rectangle>>(new[] { new Rectangle(capture.Window.Left + 200, capture.Window.Top + 180, 250, 120) })) : null;
                    // Only desktop pixels and window candidates are synthetic;
                    // production owns settings, acquisition order and outputs.
                    if (Environment.GetEnvironmentVariable("WINONE_PROBE_EMPTY_SELECTION") != "1") {
                        capture.SetRegion(new Rectangle(100, 100, 700, 400));
                        capture.AddRectangle(new Rectangle(200, 200, 150, 100));
                    }
                    using var observer = new NativeSessionProbe(capture, nativeSession, session, clock, ShareX.HelpersLib.ShareXResources.Theme.IsDarkTheme);
                    capture.ShowDialog();
                    if (nativeSession.Pinned) {
                        var pin = Application.OpenForms.OfType<ShareX.PinToScreenForm>().Single();
                        if (!pin.Visible || !pin.TopMost || pin.Image.Size != new Size(700, 400))
                            throw new InvalidOperationException("Pinned image is not visible, topmost, or correctly sized");
                    }
                });
            } finally {
                File.WriteAllText(Path.Combine(session, "managed-metrics.json"), JsonSerializer.Serialize(new {
                    estimatedManagedBytes = GC.GetTotalMemory(false), gen0 = GC.CollectionCount(0), gen1 = GC.CollectionCount(1), gen2 = GC.CollectionCount(2)
                }));
            }
        }, bitmap => {
            copies++;
            bitmap.Save(Path.Combine(output, "copied.png"));
            File.WriteAllText(Path.Combine(output, "copy.json"), JsonSerializer.Serialize(new { copies, bitmap.Width, bitmap.Height, realClipboard = false }));
        });
        File.WriteAllText(Path.Combine(output, "worker-ready.json"), JsonSerializer.Serialize(new { pipe, processId = Environment.ProcessId }));
        Application.Run(worker);
        return worker.ExitCode;
    }
}
