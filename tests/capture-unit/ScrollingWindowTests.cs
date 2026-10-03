using ShareX.HelpersLib;
using WinOne.Capture;

internal static class ScrollingWindowTests
{
    public static void Run(string directory)
    {
        Exception? failure = null;
        var thread = new Thread(() =>
        {
            try
            {
                Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
                Application.EnableVisualStyles();
                Directory.CreateDirectory(directory);
                foreach (var language in new[] { "en-US", "zh-CN" })
                {
                    var english = language == "en-US";
                    ShareXResources.Theme = english ? ShareXTheme.DarkTheme : ShareXTheme.LightTheme;
                    var selected = 0;
                    using var window = new ScrollingWindow(new CaptureSettings(), language, _ => { selected++; return false; });
                    var start = (Button)window.Controls.Find("scroll-start", true).Single();
                    var edit = (Button)window.Controls.Find("scroll-edit", true).Single();
                    var accept = (Button)window.Controls.Find("scroll-use", true).Single();
                    if (edit.Enabled || accept.Enabled) throw new Exception("empty scrolling result can be accepted");
                    if (start.Text != (english ? "Select area" : "选择区域") || edit.Text != (english ? "Edit" : "编辑")) throw new Exception("scroll controls ignored language");
                    var source = new Bitmap(700, 320);
                    {
                        using var graphics = Graphics.FromImage(source);
                        graphics.Clear(Color.SlateBlue);
                        graphics.DrawString("WinOne · Scrolling capture", window.Font, Brushes.White, 30, 30);
                        window.OwnPreview(source);
                    }
                    Exception? runFailure = null;
                    window.Shown += async (_, _) =>
                    {
                        try
                        {
                            start.PerformClick();
                            var deadline = Environment.TickCount64 + 5000;
                            while (!start.Enabled && Environment.TickCount64 < deadline) await Task.Delay(25);
                            if (!start.Enabled || selected != 1) throw new Exception("selection did not finish");
                            if (!edit.Enabled || !accept.Enabled) throw new Exception("cancelled reselection discarded result actions");
                            if (source.Size != new Size(700, 320) || source.GetPixel(0, 0).ToArgb() != Color.SlateBlue.ToArgb()) throw new Exception("cancelled reselection changed result");
                            using var screenshot = new Bitmap(window.Width, window.Height);
                            window.DrawToBitmap(screenshot, new Rectangle(Point.Empty, screenshot.Size));
                            screenshot.Save(Path.Combine(directory, $"scroll-{language}.png"));
                            edit.PerformClick();
                        }
                        catch (Exception error) { runFailure = error; window.Close(); }
                    };
                    var outcome = window.ShowDialog();
                    if (outcome != DialogResult.OK || !window.EditRequested) throw runFailure ?? new Exception($"edit action did not hand off the capture: result={outcome}, selected={selected}, startEnabled={start.Enabled}, disposed={window.IsDisposed}");
                    if (runFailure is not null) throw runFailure;
                    using var retained = window.TakeResult() ?? throw new Exception("cancelled reselection lost result");
                    if (!ReferenceEquals(retained, source) || window.TakeResult() is not null) throw new Exception("window transfer cloned or retained the bitmap");
                    window.Dispose();
                    if (retained.GetPixel(0, 0).ToArgb() != Color.SlateBlue.ToArgb()) throw new Exception("closing preview invalidated the transferred image");
                }
                using var replacementWindow = new ScrollingWindow(new CaptureSettings());
                var first = new Bitmap(20, 30);
                var second = new Bitmap(24, 32);
                replacementWindow.OwnPreview(first);
                replacementWindow.OwnPreview(first);
                _ = first.GetPixel(0, 0);
                replacementWindow.OwnPreview(second);
                RequireDisposed(first);
                replacementWindow.Dispose();
                RequireDisposed(second);
                var late = new Bitmap(10, 10);
                try { replacementWindow.OwnPreview(late); throw new Exception("disposed window accepted ownership"); }
                catch (ObjectDisposedException) { }
                RequireDisposed(late);
            }
            catch (Exception error) { failure = error; }
        });
        thread.SetApartmentState(ApartmentState.STA); thread.Start();
        if (!thread.Join(TimeSpan.FromSeconds(20))) throw new Exception("scroll UI test timed out");
        if (failure is not null) throw failure;
        Console.WriteLine("PASS: bilingual scroll UI, cancelled reselection, zero-copy handoff, replacement and disposal ownership");
    }
    private static void RequireDisposed(Bitmap image)
    {
        try { _ = image.GetPixel(0, 0); throw new Exception("released bitmap remained usable"); }
        catch (ArgumentException) { }
    }
}
