using System.Diagnostics;
using System.Text.Json;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

internal static class Program
{
    [STAThread]
    private static int Main(string[] args)
    {
        if (args.Length is < 1 or > 2) return 2;
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
        Application.EnableVisualStyles();
        var output = Path.GetFullPath(args[0]); Directory.CreateDirectory(output);
        try {
            GeometryAndToolbar();
            var engine = new NativeRecordingEngine();
            Check(engine.CallAsync("status").GetAwaiter().GetResult().Phase == "idle", "Native ABI did not return idle");
            if (args.Length == 1) { Console.WriteLine("PASS: physical selection, format defaults, fixed toolbar ordering, edit guards and native ABI"); return 0; }
            if (args[1] != "--live") return 2;
            // Explicit system test: record only this opaque, owned fixture. The
            // desktop outside its client rectangle never enters the saved file.
            using var fixture = new Fixture { StartPosition = FormStartPosition.Manual, ClientSize = new Size(641, 361), Text = "WinOne recording test target", TopMost = true };
            var screen = Screen.PrimaryScreen!.WorkingArea;
            // Keep this opt-in fixture away from the usual upper-left work
            // area/context menus; the guard still aborts on any obstruction.
            fixture.Location = new Point(Math.Max(screen.Left + 24, screen.Right - fixture.Width - 24),
                Math.Max(screen.Top + 24, screen.Bottom - fixture.Height - 24));
            // A second, differently colored owned window is the only surface
            // beneath the fixture. If capture exclusion removes too much, the
            // decoder sees a test failure, not the user's desktop contents.
            using var backdrop = new Form { FormBorderStyle = FormBorderStyle.None, BackColor = Color.Lime,
                Bounds = Rectangle.Inflate(fixture.Bounds, 24, 24), TopMost = true, ShowInTaskbar = false,
                StartPosition = FormStartPosition.Manual };
            backdrop.Show();
            Exception? failure = null;
            fixture.Shown += (_, _) => fixture.BeginInvoke((Action)(async () => {
                try {
                    await Live(engine, fixture, output);
                    await RecordingUiTests.RunLive(engine, fixture, output, () => { fixture.Frame++; fixture.Invalidate(); });
                    await RecordingUiTests.Run(fixture, Path.Combine(output, "ui"));
                }
                catch (Exception error) {
                    failure = error;
                    // Discard a failed test recording, particularly if the
                    // fixture guard detected another window during capture.
                    try { await engine.CallAsync("cancel_recording"); }
                    catch (Exception cleanup) { Console.Error.WriteLine("Test recording cleanup: " + cleanup.Message); }
                }
                finally { fixture.Close(); }
            }));
            Application.Run(fixture);
            if (failure is not null) System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(failure).Throw();
            Console.WriteLine("PASS: WGC MP4/GIF, odd crop, pause compensation, static tail, cancel and completed-file preservation");
            return 0;
        } catch (Exception error) { Console.Error.WriteLine(error); File.WriteAllText(Path.Combine(output, "failure.txt"), error.ToString()); return 1; }
    }
    private static void Check(bool value, string message) { if (!value) throw new InvalidOperationException(message); }
    private static void GeometryAndToolbar()
    {
        var stamp = new DateTime(2026, 10, 18, 12, 34, 56);
        Check(CaptureFileName.RenderRecording(null, stamp, "mp4") == "WinOne-20261018123456-Video-OK.mp4", "Default video type marker is missing");
        Check(CaptureFileName.RenderRecording("WinOne-{date}-OK.png", stamp, "gif") == "WinOne-20261018-GIF-OK.gif", "GIF marker is not before OK");
        Check(CaptureFileName.RenderRecording("项目-{date}", stamp, "gif") == "项目-20261018-GIF.gif", "Custom filename template was replaced");
        var config = RecordingConfig.FromSelection(new Rectangle(-1519, 71, 191, 111), "display", "test.gif", "gif", 1.5);
        Check((config.X, config.Y, config.Width, config.Height) == (-1519, 71, 190, 110), "DPI was applied twice or physical origin changed");
        Check(config.Fps == 15 && config.Quality == 85 && config.Coordinates == "physicalDesktop", "GIF defaults changed");
        try { RecordingConfig.FromSelection(new Rectangle(0, 0, 15, 16), "display", "test.mp4", "mp4", 1); throw new Exception("Tiny region accepted"); } catch (InvalidOperationException) { }
        using var toolbar = new NativeToolbarControl(toolbarTools: []);
        var tools = toolbar.Controls.OfType<ToolStrip>().Single(row => row.Name == "capture-tools");
        var names = tools.Items.Cast<ToolStripItem>().Select(item => item.Name).ToArray();
        var scroll = Array.IndexOf(names, "scroll");
        Check(names[scroll + 1] == "video" && names[scroll + 2] == "gif", "Recording icons are not directly after scrolling capture");
        toolbar.SetRecordingAvailable(true); toolbar.SetState(false, false, "select", false);
        Check(tools.Items["video"]!.Enabled && tools.Items["gif"]!.Enabled, "Recording entry is disconnected");
        Check(tools.Items["video"]!.Overflow != ToolStripItemOverflow.Always, "User annotation preferences hid the fixed recording action");
        toolbar.SetState(false, false, "text", false, true);
        Check(!tools.Items["video"]!.Enabled && !tools.Items["gif"]!.Enabled, "Recording bypassed text-edit protection");
    }
    private static async Task Live(NativeRecordingEngine engine, Fixture fixture, string output)
    {
        var evidence = new List<object>();
        var directory = Path.Combine(output, "clips"); Directory.CreateDirectory(directory);
        foreach (var format in new[] { "mp4", "gif" }) {
            fixture.Activate(); fixture.BringToFront(); fixture.Update();
            // The toolbar deliberately overlaps the target. Exclusion must
            // expose the fixture beneath, not a black rectangle or controls.
            using var controls = new Form { Text = "WinOne excluded recording controls", BackColor = Color.Magenta, ClientSize = new Size(160, 50), TopMost = true, ShowInTaskbar = false, StartPosition = FormStartPosition.Manual };
            controls.Location = fixture.PointToScreen(new Point(280, 145));
            RecordingWindowProtection.Exclude(controls.Handle); controls.Show();
            var rect = fixture.RectangleToScreen(fixture.ClientRectangle);
            using var outline = new NativeRecordingRegion(new Rectangle(rect.X, rect.Y, rect.Width & ~1, rect.Height & ~1));
            outline.Show();
            var config = RecordingConfig.FromSelection(rect, Screen.FromRectangle(rect).DeviceName, Path.Combine(directory, "live." + format), format, fixture.DeviceDpi / 96d) with {
                ExcludeWindows = [unchecked((ulong)controls.Handle.ToInt64()), unchecked((ulong)outline.Handle.ToInt64())], Cursor = false,
            };
            RecordingFixtureGuard.Arrange(fixture, rect, controls, outline);
            var clock = Stopwatch.StartNew();
            var started = await engine.CallAsync("start_recording", config); Check(started.Active, "WGC did not start");
            for (var i = 0; i < 16; i++) { fixture.Frame++; fixture.Invalidate(); await RecordingFixtureGuard.Wait(fixture, rect, 80, controls, outline); }
            var before = await engine.CallAsync("pause_recording");
            await RecordingFixtureGuard.Wait(fixture, rect, 950, controls, outline);
            var paused = await engine.CallAsync("status"); Check(paused.Phase == "paused" && paused.DurationMs == before.DurationMs, "Paused time leaked into the recording clock");
            await engine.CallAsync("resume_recording");
            fixture.Frame++; fixture.Invalidate(); await RecordingFixtureGuard.Wait(fixture, rect, 550, controls, outline);
            var staticStart = await engine.CallAsync("status");
            await RecordingFixtureGuard.Wait(fixture, rect, 2300, controls, outline);
            var saved = await engine.CallAsync("stop_recording"); clock.Stop();
            Check(saved.Phase == "completed", saved.Error ?? "WGC did not finalize");
            Check(saved.Width == 640 && saved.Height == 360 && saved.Bytes > 0 && File.Exists(saved.Path), "Invalid cropped output");
            Check(saved.DurationMs >= 3800 && saved.DurationMs < (ulong)clock.ElapsedMilliseconds - 650, "Pause compensation or static tail was lost");
            Check(saved.MaxQueue <= 8, "Input queue grew past its capacity");
            Check(saved.EncodedFrames - staticStart.EncodedFrames < 12, "Idle screen was encoded at a fixed rate");
            evidence.Add(new { format, wallMs = clock.ElapsedMilliseconds, saved });
            // Cancelling after completion must never delete a finished file.
            var completedPath = saved.Path!;
            await engine.CallAsync("cancel_recording"); Check(File.Exists(completedPath), "A completed file was deleted by cancel");
        }
        var region = fixture.RectangleToScreen(fixture.ClientRectangle);
        var cancelledPath = Path.Combine(directory, "cancelled.gif");
        var cancelConfig = RecordingConfig.FromSelection(region, Screen.FromRectangle(region).DeviceName, cancelledPath, "gif", 1) with { Cursor = false };
        RecordingFixtureGuard.Arrange(fixture, region);
        await engine.CallAsync("start_recording", cancelConfig);
        fixture.Frame++; fixture.Invalidate(); await RecordingFixtureGuard.Wait(fixture, region, 400);
        var cancelled = await engine.CallAsync("cancel_recording");
        Check(cancelled.Phase == "cancelled" && !File.Exists(cancelledPath) && Directory.GetFiles(directory, "*.partial").Length == 0, "Cancel left an unfinished file or worker");
        evidence.Add(new { cancelled });
        File.WriteAllText(Path.Combine(output, "live-results.json"), JsonSerializer.Serialize(evidence, new JsonSerializerOptions(JsonSerializerDefaults.Web) { WriteIndented = true }));
    }
    private sealed class Fixture : Form
    {
        public int Frame;
        public Fixture() { DoubleBuffered = true; BackColor = Color.White; }
        protected override void OnPaint(PaintEventArgs e)
        {
            e.Graphics.Clear(Color.White);
            e.Graphics.FillRectangle(Brushes.Red, 0, 0, 180, 90);
            e.Graphics.FillRectangle(Brushes.Blue, 0, 270, 180, 90);
            e.Graphics.FillRectangle(Brushes.LightGray, 230, 120, 210, 120);
            e.Graphics.DrawString("WinOne · Live capture", Font, Brushes.Black, 200, 25);
            e.Graphics.DrawString("Only this test window is recorded", Font, Brushes.Black, 200, 65);
            e.Graphics.DrawString("Frame " + Frame, Font, Brushes.Black, 260, 160);
            e.Graphics.FillRectangle(Frame % 2 == 0 ? Brushes.Black : Brushes.DarkGray, 200, 280, 340, 40);
        }
    }
}
