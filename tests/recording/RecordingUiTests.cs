using System.Text.Json;
using System.Runtime.InteropServices;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

internal static class RecordingUiTests
{
    [DllImport("user32.dll")] private static extern bool GetWindowDisplayAffinity(IntPtr window, out uint affinity);
    // Only I/O is replaced here. Production WinForms controls, state changes,
    // layout and the original editor's visibility/close protection are real.
    internal static async Task Run(Form capture, string output)
    {
        Directory.CreateDirectory(output);
        foreach (var dark in new[] { false, true }) foreach (var english in new[] { false, true }) {
            var settings = new CaptureSettings { Theme = dark ? "dark" : "light", OutputDirectory = output };
            CaptureOperations.ApplyTheme(settings);
            var engine = new Engine(); string? selectedPath = null;
            using var window = new NativeRecordingWindow(capture, new Rectangle(60, 60, 640, 360), settings, "gif", english ? "en-US" : "zh-CN", engine, _ => selectedPath);
            window.Show(); await Task.Delay(30);
            var actions = window.Controls.OfType<TableLayoutPanel>().Single().Controls.OfType<ToolStrip>().Single();
            ToolStripItem Button(string name) => actions.Items.Cast<ToolStripItem>().Single(item => item.Name == "recording-" + name);
            void Check(bool value, string message) { if (!value) throw new Exception(message); }
            async Task Click(string name) { Button(name).PerformClick(); await window.CurrentOperation; }
            var prefix = (dark ? "dark" : "light") + (english ? "-en" : "-zh");
            void Shot(string phase) {
                window.Update();
                using var image = new Bitmap(window.Width, window.Height);
                window.DrawToBitmap(image, new Rectangle(Point.Empty, window.Size));
                image.Save(Path.Combine(output, prefix + "-" + phase + ".png"));
                foreach (var button in actions.Items.Cast<ToolStripItem>().Where(item => item.Available))
                    Check(button.Placement == ToolStripItemPlacement.Main && actions.ClientRectangle.Contains(button.Bounds), "Recording control was clipped: " + button.Name);
            }
            Shot("setup");
            Check(window.TopMost && ((ToolStripButton)Button("pin")).Checked, "Pinned recorder lost its visible pin state");
            Button("pin").PerformClick(); Check(!window.TopMost && !((ToolStripButton)Button("pin")).Checked, "Unpin did not change the native window state");
            Button("pin").PerformClick(); Check(window.TopMost, "Pin did not restore native topmost state");
            await Click("start"); Check(engine.Calls.Count == 0 && capture.Visible, "Cancelled picker hid the screenshot or started recording");
            selectedPath = Path.Combine(output, prefix + ".gif");
            engine.FailStart = true; await Click("start");
            Check(!capture.Visible && window.Visible && Button("start").Available, "Failed start covered the retry panel with the frozen editor");
            engine.FailStart = false; await Click("start");
            Check(!capture.Visible && Button("pause").Available && Button("stop").Available, "Start did not switch to recording controls");
            Check(window.RegionWindow is { Visible: true } outline && outline.Bounds == new Rectangle(59, 59, 642, 362), "Recording lost or moved its region outline");
            Check(GetWindowDisplayAffinity(window.RegionWindow!.Handle, out var affinity) && affinity == 0x11, "Recording outline was not excluded from capture");
            var shape = window.RegionWindow.Region;
            Check(shape is not null && !shape.IsVisible(320, 180) && shape.IsVisible(0, 180) && shape.IsVisible(641, 180)
                && shape.IsVisible(320, 0) && shape.IsVisible(320, 361), "The outline must have a real hole, not a color-keyed surface over the recording");
            Shot("recording");
            await Click("pause"); Check(window.RegionWindow?.Visible == true, "Paused recording lost its selected area"); Check(Button("pause").Text == (english ? "Resume" : "继续"), "Paused control did not expose resume"); Shot("paused");
            await Click("pause");
            await Click("stop"); Check(window.Result?.Phase == "completed" && Button("folder").Available, "Saved output was not presented"); Shot("saved");
            Check(engine.Config is { Format: "gif", Fps: 15, Quality: 85, ExcludeWindows.Length: 3 }, "UI dropped recording configuration or exclusion handles");
            Check(window.RegionWindow is null, "Finished recording retained the outline");
            Button("pin").PerformClick(); capture.Show(); capture.Activate();
            var closeDeadline = DateTime.UtcNow.AddSeconds(3);
            while (!window.IsDisposed && DateTime.UtcNow < closeDeadline) await Task.Delay(10);
            Check(window.IsDisposed, "Unpinned completed panel stayed open after outside activation");
            File.WriteAllText(Path.Combine(output, prefix + ".json"), JsonSerializer.Serialize(engine.Calls));
        }
        await VerifyColdPanelHandoff(capture, output);
        await VerifyActiveOutsideClose(capture, output);
    }
    // Exercise the production panel -> config -> native DLL path, not just a
    // stand-alone WGC session plus a separately mocked set of controls.
    internal static async Task RunLive(IRecordingEngine engine, Form fixture, string output, Action advanceFrame)
    {
        var evidence = new List<object>();
        foreach (var format in new[] { "mp4", "gif" }) {
            var rect = fixture.RectangleToScreen(fixture.ClientRectangle);
            using var editor = new Form { Text = "WinOne test screenshot editor", StartPosition = FormStartPosition.Manual,
                Bounds = fixture.Bounds, BackColor = Color.Silver, TopMost = true, ShowInTaskbar = false };
            editor.Show();
            var path = Path.Combine(output, "clips", "panel." + format);
            using var window = new NativeRecordingWindow(editor, rect, new CaptureSettings { OutputDirectory = output }, format, "zh-CN", engine, _ => path);
            window.Show();
            await RecordingInput.Until(() => window.RegionWindow is { Visible: true }, "Recording setup never presented its selected area");
            var actions = window.Controls.OfType<TableLayoutPanel>().Single().Controls.OfType<ToolStrip>().Single();
            await RecordingInput.ClickInactive(fixture, window, actions, "start",
                () => RecordingFixtureGuard.Arrange(fixture, rect, window, window.RegionWindow!),
                () => actions.Items["recording-pause"]!.Available);
            await window.CurrentOperation;
            var outline = window.RegionWindow ?? throw new Exception("The production recording panel did not create an outline");
            if (editor.Visible || !outline.Visible) throw new Exception("The editor/outline handoff failed");
            // Keep both controls and outline over the generated target. Their
            // WDA exclusions must reveal the target in every sampled frame.
            // Default placement was outside; deliberately drag it over this
            // generated target now to verify exclusion of controls AND hints.
            window.Location = fixture.PointToScreen(new Point(50, 70));
            RecordingFixtureGuard.Arrange(fixture, rect, window, outline);
            for (var i = 0; i < 8; i++) {
                advanceFrame();
                await RecordingFixtureGuard.Wait(fixture, rect, 80, window, outline);
            }
            RecordingInput.Hover(window, actions, "pause");
            Rectangle tooltip;
            try { tooltip = await RecordingInput.WaitForProtectedTooltip(); }
            catch {
                using var image = new Bitmap(window.Width, window.Height);
                window.DrawToBitmap(image, new Rectangle(Point.Empty, window.Size));
                image.Save(Path.Combine(output, "hover-failure.png"));
                Console.Error.WriteLine("Hover pointer=" + Cursor.Position + "; panel=" + window.Bounds + "; strip=" + actions.RectangleToScreen(actions.ClientRectangle));
                Console.Error.WriteLine("Owned thread windows: " + RecordingInput.DescribeThreadWindows());
                foreach (var label in window.Controls.OfType<TableLayoutPanel>().Single().Controls.OfType<Label>()) Console.Error.WriteLine("Panel label: " + label.Text);
                throw;
            }
            if (!tooltip.IntersectsWith(rect)) throw new Exception("Tooltip exclusion was not exercised over the recorded target");
            var tooltipArea = Rectangle.Inflate(tooltip, 4, 4);
            tooltipArea.Offset(-rect.X, -rect.Y);
            // This part of the generated target is plain white, including the
            // native hint's shadow margin. Decode pixels here, not just beside
            // the panel, so tooltip/black-hole leaks cannot pass unnoticed.
            var tooltipWhiteArea = Rectangle.Intersect(tooltipArea, new Rectangle(0, 96, 180, 168));
            if (tooltipWhiteArea.Width < 16 || tooltipWhiteArea.Height < 12) throw new Exception("The tooltip did not cover enough of the fixture's white reference area");
            var tooltipStartMs = (await engine.CallAsync("status")).DurationMs;
            advanceFrame();
            await RecordingFixtureGuard.Wait(fixture, rect, 400, window, outline);
            var tooltipEndMs = (await engine.CallAsync("status")).DurationMs;
            await RecordingInput.ClickInactive(fixture, window, actions, "pause",
                () => RecordingFixtureGuard.Arrange(fixture, rect, window, outline), () => actions.Items["recording-pause"]!.Text == "继续");
            if (!outline.Visible) throw new Exception("The production panel lost its outline while paused");
            await RecordingFixtureGuard.Wait(fixture, rect, 350, window, outline);
            await RecordingInput.ClickInactive(fixture, window, actions, "pause",
                () => RecordingFixtureGuard.Arrange(fixture, rect, window, outline), () => actions.Items["recording-pause"]!.Text == "暂停");
            await RecordingFixtureGuard.Wait(fixture, rect, 1250, window, outline);
            await RecordingInput.ClickInactive(fixture, window, actions, "stop",
                () => RecordingFixtureGuard.Arrange(fixture, rect, window, outline), () => window.Result?.Phase == "completed");
            await window.CurrentOperation;
            var saved = window.Result ?? throw new Exception("The production panel did not save its recording");
            if (saved.Phase != "completed" || !File.Exists(path) || window.RegionWindow is not null)
                throw new Exception("The production recording panel did not finalize/clear its outline");
            evidence.Add(new { format, saved, tooltip, tooltipWhiteArea, tooltipStartMs, tooltipEndMs, inactiveSingleClicks = new[] { "start", "pause", "resume", "stop" } });
            window.Close();
        }
        File.WriteAllText(Path.Combine(output, "panel-results.json"), JsonSerializer.Serialize(evidence, new JsonSerializerOptions(JsonSerializerDefaults.Web) { WriteIndented = true }));
    }
    private static async Task VerifyColdPanelHandoff(Form capture, string output)
    {
        var settings = new CaptureSettings { OutputDirectory = output };
        for (var attempt = 0; attempt < 3; attempt++) {
            Form? shown = null;
            var lifetime = NativeRecordingWindow.RunAsync(capture, new Rectangle(60, 60, 640, 360), settings, "mp4", "zh-CN", CancellationToken.None, new Engine(), window => shown = window);
            if (shown is not { Visible: true } || capture.Visible) throw new Exception("Cold recording setup left its frozen screenshot over the panel");
            shown.Close();
            if (await lifetime is not null || !capture.Visible) throw new Exception("Canceling recording setup lost the original editor");
        }
        try {
            await NativeRecordingWindow.RunAsync(capture, new Rectangle(60, 60, 640, 360), settings, "mp4", "zh-CN", CancellationToken.None, new Engine(), _ => throw new IOException("Test presentation failure"));
            throw new Exception("Test startup failure was not surfaced");
        } catch (IOException) {
            if (!capture.Visible) throw new Exception("Failed first panel presentation left the editor hidden");
        }
    }
    private static async Task VerifyActiveOutsideClose(Form capture, string output)
    {
        var engine = new Engine();
        using var window = new NativeRecordingWindow(capture, new Rectangle(60, 60, 640, 360),
            new CaptureSettings { OutputDirectory = output }, "mp4", "zh-CN", engine, _ => Path.Combine(output, "outside-close.mp4"));
        window.Show(); window.Activate();
        var actions = window.Controls.OfType<TableLayoutPanel>().Single().Controls.OfType<ToolStrip>().Single();
        actions.Items["recording-start"]!.PerformClick();
        await window.CurrentOperation;
        if (window.RegionWindow?.Visible != true) throw new Exception("Active close fixture never started recording");
        actions.Items["recording-pin"]!.PerformClick();
        capture.Show(); capture.Activate();
        var deadline = DateTime.UtcNow.AddSeconds(3);
        while (!window.IsDisposed && DateTime.UtcNow < deadline) await Task.Delay(10);
        if (!window.IsDisposed || window.Result?.Phase != "completed" || !engine.Calls.Contains("stop_recording") || engine.Calls.Contains("cancel_recording"))
            throw new Exception("Unpinned outside close did not safely stop/save the active recording");
        File.WriteAllText(Path.Combine(output, "outside-close.json"), JsonSerializer.Serialize(new { saved = true, closed = true, calls = engine.Calls }));
    }
    private sealed class Engine : IRecordingEngine
    {
        internal bool FailStart;
        internal readonly List<string> Calls = [];
        internal RecordingConfig? Config;
        private RecordingStatus status = new();
        public Task<RecordingStatus> CallAsync(string method, RecordingConfig? config = null)
        {
            Calls.Add(method);
            if (method == "start_recording" && FailStart) return Task.FromException<RecordingStatus>(new IOException("Test save destination unavailable"));
            if (config is not null) Config = config;
            status = status with { Phase = method switch { "start_recording" or "resume_recording" => "recording", "pause_recording" => "paused", "stop_recording" => "completed", "cancel_recording" => "cancelled", _ => status.Phase },
                Format = "gif", DurationMs = 6500, EncodedFrames = 12, Bytes = 126000, Path = Config?.Output, Width = 640, Height = 360 };
            return Task.FromResult(status);
        }
    }
}
