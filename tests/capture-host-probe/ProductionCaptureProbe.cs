using System.Diagnostics;
using System.IO.Pipes;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;

internal static class ProductionCaptureProbe
{
    private delegate bool EnumProc(IntPtr window, IntPtr data);
    [DllImport("user32.dll")] private static extern bool EnumWindows(EnumProc callback, IntPtr data);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr window, out uint process);
    [DllImport("user32.dll")] private static extern bool IsWindowVisible(IntPtr window);
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    [DllImport("user32.dll")] private static extern uint GetGuiResources(IntPtr process, uint kind);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] private static extern int GetClassNameW(IntPtr window, StringBuilder text, int count);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] private static extern int GetWindowTextW(IntPtr window, StringBuilder text, int count);

    public static int Run(string executable, string directory, int iterations = 1)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private test desktop");
        if (Directory.Exists(directory)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(directory); directory = Path.GetFullPath(directory);
        Application.EnableVisualStyles();
        using var image = new Bitmap(800, 600);
        using (var graphics = Graphics.FromImage(image)) graphics.Clear(Color.LightBlue);
        using var canvas = new WinOneCaptureCanvas(image);
        canvas.SetRegion(new Rectangle(50, 50, 700, 500));
        canvas.AddRectangle(new Rectangle(120, 100, 160, 120));
        var settings = new SettingsStore(directory);
        settings.Save(settings.Snapshot() with { CopyToClipboard = false, OutputDirectory = Path.Combine(directory, "exports") });
        var drafts = new NativeCaptureDraftStore(directory);
        var oldBounds = new Rectangle(-1920, 0, image.Width, image.Height);
        void Seed() {
            var draft = drafts.Begin(image, settings.Snapshot(), canvas.ExportDocument(), "region", CapturePlacement.From(oldBounds, oldBounds));
            if (iterations > 1) drafts.Update(draft.Id, draft.Revision, draft.Document,
                new NativeTextInputState(null, "WinOne 连续恢复\n文字输入", 180, 250, 400, 70, 300, 24, Color.Red.ToArgb(), 2, 3));
        }
        Seed();
        var pipeName = "WinOne-Production-Probe-" + Guid.NewGuid().ToString("N");
        var start = new ProcessStartInfo(Path.GetFullPath(executable)) { UseShellExecute = false, CreateNoWindow = true, RedirectStandardError = true };
        foreach (var argument in new[] { "--pipe", pipeName, "--data-dir", directory, "--parent", Environment.ProcessId.ToString() }) start.ArgumentList.Add(argument);
        using var worker = Process.Start(start) ?? throw new Exception("Could not start production worker");
        var errors = worker.StandardError.ReadToEndAsync();
        JsonElement Call(string method, object? parameters = null) => Task.Run(async () =>
        {
            // The fixture's hidden canvas installs a WinForms context. Pipe
            // awaits must run off that blocked STA, not wait for its UI pump.
            using var timeout = new CancellationTokenSource(TimeSpan.FromSeconds(5));
            using var pipe = new NamedPipeClientStream(".", pipeName, PipeDirection.InOut, PipeOptions.Asynchronous);
            await pipe.ConnectAsync(timeout.Token);
            await PipeProtocol.WriteAsync(pipe, JsonSerializer.SerializeToUtf8Bytes(new { version = 1, id = Guid.NewGuid().ToString("N"), method, @params = parameters ?? new { } }), timeout.Token);
            using var response = JsonDocument.Parse(await PipeProtocol.ReadAsync(pipe, timeout.Token));
            if (response.RootElement.TryGetProperty("error", out var error) && error.ValueKind != JsonValueKind.Null) throw new Exception(error.ToString());
            return response.RootElement.GetProperty("result").Clone();
        }).GetAwaiter().GetResult();
        try {
            if (Call("snapshot").GetProperty("processId").GetInt32() != worker.Id) throw new Exception("Wrong production worker");
            var samples = new List<object>();
            IntPtr window = IntPtr.Zero;
            for (var iteration = 0; iteration < iterations; iteration++) {
            if (iteration > 0) Seed();
            var elapsed = Stopwatch.StartNew();
            var job = Call("capture.unified", new { mode = "window" }).GetProperty("id").GetString()!;
            window = IntPtr.Zero;
            if (!SpinWait.SpinUntil(() => {
                if (worker.HasExited) throw new Exception("Worker exited: " + errors.GetAwaiter().GetResult());
                EnumWindows((candidate, _) => {
                    GetWindowThreadProcessId(candidate, out var pid);
                    if (pid != worker.Id || !IsWindowVisible(candidate)) return true;
                    var name = new StringBuilder(256); GetClassNameW(candidate, name, name.Capacity);
                    // Native EDIT can create a visible Windows input indicator.
                    // Target the capture form, never that auxiliary IME window.
                    if (name.ToString().StartsWith("WindowsForms10.Window.", StringComparison.Ordinal)) window = candidate;
                    return true;
                }, IntPtr.Zero);
                return window != IntPtr.Zero;
            }, TimeSpan.FromSeconds(10))) throw new Exception("Production entry did not open a native recovery window");
            // Only a synthetic recovered image is opened. Right-click sends no
            // clipboard command and never invokes desktop frame acquisition.
            var repeated = Call("capture.unified", new { mode = "window" });
            if (repeated.GetProperty("id").GetString() != job) throw new Exception("Repeated capture replaced the active job");
            if (!Call("capture.activate").GetProperty("active").GetBoolean()) throw new Exception("Active capture could not be reactivated");
            var className = new StringBuilder(256); var title = new StringBuilder(256);
            GetClassNameW(window, className, className.Capacity); GetWindowTextW(window, title, title.Capacity);
            File.AppendAllText(Path.Combine(directory, "trace.jsonl"), JsonSerializer.Serialize(new { iteration, window = window.ToInt64(), windowClass = className.ToString(), title = title.ToString() }) + "\n");
            SendMessageW(window, 0x0205, IntPtr.Zero, IntPtr.Zero);
            JsonElement result = default;
            if (!SpinWait.SpinUntil(() => { result = Call("jobs.get", new { id = job }); return result.GetProperty("state").GetString() != "running"; }, TimeSpan.FromSeconds(10)))
                throw new Exception("Production capture did not release its busy state");
            if (result.GetProperty("state").GetString() != "cancelled" || drafts.Read() is not null || drafts.ReadReservation() is not null)
                throw new Exception("Production cancellation retained its recovery draft: " + result);
            if (Call("capture.activate").GetProperty("active").GetBoolean()) throw new Exception("Reactivation after cancellation started another capture");
            worker.Refresh();
            samples.Add(new { iteration = iteration + 1, privateBytes = worker.PrivateMemorySize64, workingSetBytes = worker.WorkingSet64,
                handles = worker.HandleCount, gdi = GetGuiResources(worker.Handle, 0), user = GetGuiResources(worker.Handle, 1), recoveryAndCancelMs = elapsed.Elapsed.TotalMilliseconds });
            }
            Call("shutdown");
            if (!worker.WaitForExit(10000) || worker.ExitCode != 0) throw new Exception("Production worker failed to exit");
            File.WriteAllText(Path.Combine(directory, "production-verification.json"), JsonSerializer.Serialize(new { productionEntry = true, worker = worker.Id, nativeWindow = window.ToInt64(), cancelled = true, draftRetired = true, exitCode = worker.ExitCode, desktopCaptured = false, clipboardWritten = false }));
            File.WriteAllText(Path.Combine(directory, "resources.json"), JsonSerializer.Serialize(new { iterations, samples, scope = "One production screenshot worker; synthetic recovery and cancellation with pending text; no forced GC, real desktop acquisition, Host or WebView2" }));
            Console.WriteLine("PASS: actual production worker restores a synthetic draft through the default native entry, cancels and exits");
            return 0;
        } finally {
            // This exact child is owned by this fixture. Never enumerate or stop
            // user WinOne processes when a test fails.
            if (!worker.HasExited) { worker.Kill(); worker.WaitForExit(); }
            File.WriteAllText(Path.Combine(directory, "worker-stderr.txt"), errors.GetAwaiter().GetResult());
        }
    }
}
