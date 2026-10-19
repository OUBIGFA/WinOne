using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text.Json;
using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

// Real HWND/message-pump timing on a private desktop with generated pixels.
// It does not claim physical hotkey-to-photon latency or a cold OS disk cache.
internal static class NativeCaptureLatencyProbe
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    [DllImport("psapi.dll", SetLastError = true)] private static extern bool EmptyWorkingSet(IntPtr process);
    private static IntPtr At(int x, int y) => new((y << 16) | (x & 0xffff));

    // RegionCaptureForm paints without raising Control.Paint. Observe the
    // native message itself, as production readiness does.
    private sealed class FirstPaint : NativeWindow, IDisposable
    {
        private readonly Action painted;
        private bool fired;
        internal FirstPaint(Form window, Action painted) { this.painted = painted; AssignHandle(window.Handle); }
        protected override void WndProc(ref Message message)
        {
            base.WndProc(ref message);
            if (message.Msg == 0x000f && !fired) { fired = true; painted(); }
        }
        public void Dispose() => ReleaseHandle();
    }

    internal static int Run(string output)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private test desktop");
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
        Application.EnableVisualStyles();
        var store = new SettingsStore(output);
        var operations = new CaptureOperations(store, output, _ => throw new InvalidOperationException("No clipboard output in latency probe"));
        var bounds = CaptureHelpers.GetScreenBounds();
        using var process = Process.GetCurrentProcess();
        var samples = new List<object>();
        void Sample(string phase)
        {
            var clock = Stopwatch.StartNew();
            var stages = new Dictionary<string, double>();
            double firstPaint = 0, input = 0;
            Exception? failure = null;
            try {
                NativeCaptureWorkflow.Run(store, operations, "region", "zh-CN", CancellationToken.None,
                    acquire: () => {
                        var bitmap = new Bitmap(bounds.Width, bounds.Height);
                        using (var graphics = Graphics.FromImage(bitmap)) graphics.Clear(Color.White);
                        return CapturedFrame.Own(bitmap, bounds);
                    }, windows: () => [], show: (canvas, session) => {
                        var measured = false;
                        // Post after the first real WM_PAINT has returned through
                        // FirstPaintObserver, rather than forcing DrawToBitmap.
                        using var paint = new FirstPaint(canvas.Window, () => {
                            if (measured) return;
                            measured = true;
                            canvas.Window.BeginInvoke((Action)(() => {
                                try {
                                    if (!session.Ready) throw new InvalidOperationException("First paint did not make selection ready");
                                    firstPaint = clock.Elapsed.TotalMilliseconds;
                                    var watch = Stopwatch.StartNew();
                                    SendMessageW(canvas.Window.Handle, 0x0200, IntPtr.Zero, At(40, 40));
                                    SendMessageW(canvas.Window.Handle, 0x0201, new IntPtr(1), At(40, 40));
                                    SendMessageW(canvas.Window.Handle, 0x0200, new IntPtr(1), At(440, 240));
                                    SendMessageW(canvas.Window.Handle, 0x0202, IntPtr.Zero, At(440, 240));
                                    input = watch.Elapsed.TotalMilliseconds;
                                    if (canvas.Selection != new Rectangle(40, 40, 401, 201)) throw new InvalidOperationException($"First input changed inclusive pixel bounds: {canvas.Selection} ({phase})");
                                } catch (Exception error) { failure = error; }
                                finally { canvas.Window.Close(); }
                            }));
                        });
                        canvas.Window.Shown += (_, _) => stages["shown"] = clock.Elapsed.TotalMilliseconds;
                        using var timeout = new System.Windows.Forms.Timer { Interval = 5000 };
                        timeout.Tick += (_, _) => { failure = new TimeoutException("Native first paint timed out"); canvas.Window.Close(); };
                        timeout.Start();canvas.ShowDialog();timeout.Stop();
                    }, startupStage: stage => stages[stage] = clock.Elapsed.TotalMilliseconds).GetAwaiter().GetResult();
            } catch (OperationCanceledException) when (firstPaint > 0 || failure is not null) { }
            if (failure is not null) throw failure;
            if (firstPaint <= 0) throw new InvalidOperationException("No real paint was measured");
            if (Application.OpenForms.Count != 0 || operations.ActivateNativeCapture is not null) throw new InvalidOperationException("Completed capture retained a window");
            process.Refresh();
            samples.Add(new { phase, stages, firstPaintMs = firstPaint, firstInputMs = input, privateBytes = process.PrivateMemorySize64, workingSetBytes = process.WorkingSet64, handles = process.HandleCount });
        }
        var exitCode = 1;
        // Production keeps one Application.Run/dispatcher alive for every
        // capture. Ending a top-level modal loop per sample disposes WinForms'
        // synchronization context and adds artificial ToolStrip waits.
        using var dispatcher = new Form { ShowInTaskbar = false };
        _ = dispatcher.Handle;
        dispatcher.BeginInvoke(async () => {
        try {
            Sample("unprepared-first-use");
            var preparing = Stopwatch.StartNew();
            NativeCaptureStandby.Prepare(operations, store.Snapshot(), "zh-CN");
            preparing.Stop();
            var runs = int.Parse(Environment.GetEnvironmentVariable("WINONE_CAPTURE_LATENCY_RUNS") ?? "12");
            var idleSeconds = int.Parse(Environment.GetEnvironmentVariable("WINONE_CAPTURE_LATENCY_IDLE_SECONDS") ?? "65");
            if (runs is < 1 or > 100 || idleSeconds is < 0 or > 3600) throw new ArgumentOutOfRangeException("Latency probe range");
            for (var i = 0; i < runs; i++) Sample(i == 0 ? "prepared-first-use" : "warm");
            process.Refresh();var cpu = process.TotalProcessorTime;
            var idle = Stopwatch.StartNew();await Task.Delay(TimeSpan.FromSeconds(idleSeconds));idle.Stop();
            process.Refresh();var idleCpu = (process.TotalProcessorTime - cpu).TotalSeconds;
            if (idleSeconds > 0) Sample("after-idle");
            if (!EmptyWorkingSet(process.Handle)) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
            Sample("after-owned-working-set-trim");
            if (!EmptyWorkingSet(process.Handle)) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
            var resume = Stopwatch.StartNew();
            NativeCaptureStandby.Prepare(operations, store.Snapshot(), "zh-CN", refresh: true);
            resume.Stop();
            Sample("after-resume-preparation");
            File.WriteAllText(Path.Combine(output, "latency.json"), JsonSerializer.Serialize(new {
                scope = "Generated desktop-sized frame; actual private-desktop HWND/WM_PAINT and first selection input. No system suspend, real clipboard or physical input-to-photon claim.",
                screen = bounds.Size, preparationMs = preparing.Elapsed.TotalMilliseconds, idleSeconds = idle.Elapsed.TotalSeconds, idleCpuSeconds = idleCpu,
                resumePreparationMs = resume.Elapsed.TotalMilliseconds, samples, retainedWindows = Application.OpenForms.Count
            }, new JsonSerializerOptions { WriteIndented = true }));
            exitCode = 0;
        } catch (Exception error) {
            File.WriteAllText(Path.Combine(output, "failure.txt"), error.ToString());
        } finally { Application.ExitThread(); }
        });
        Application.Run();
        return exitCode;
    }
}
