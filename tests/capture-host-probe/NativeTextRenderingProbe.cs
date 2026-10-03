using System.Drawing.Imaging;
using System.Text.Json;
using ShareX.ScreenCaptureLib;

internal static class NativeTextRenderingProbe
{
    private static Bitmap RenderNative(NativeInlineText input, Bitmap background, Rectangle bounds)
    {
        using var capture = new WinOneCaptureCanvas(background);
        capture.SetRegion(bounds);
        if (!string.IsNullOrWhiteSpace(input.Text)) capture.AddText(input.Text, bounds, input.Font.Size, input.GetVisualLines(), input.ForeColor);
        return capture.RenderSelection();
    }
    public static int Run(string output)
    {
        if (Directory.Exists(output)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        using var background = new Bitmap(600, 240);
        using (var graphics = Graphics.FromImage(background)) {
            graphics.Clear(Color.White);
            using var brush = new SolidBrush(Color.FromArgb(220, 235, 250));
            for (var x = 0; x < background.Width; x += 40) graphics.FillRectangle(brush, x, 0, 20, background.Height);
        }
        using var parent = new Form { ClientSize = background.Size, ShowInTaskbar = false };
        var cases = new[] {
            ("mixed", 420, "你好，WinOne & 编辑\r\n第二行文字 and wrapping words"),
            ("wrap", 180, "中文自动换行与 English words wrapping across the selected width."),
            ("tabs", 420, "A\tB & C\r\n截图\t文字"),
            ("punctuation", 260, "WinOne-20261002-OK.png\r\n括号（内容） 123 !?"),
            ("empty", 420, ""),
            ("style-blue", 420, "文字颜色与字号\r\nWinOne inline text"),
            ("style-small", 260, "小号文字 Small text & 123"),
        };
        var results = new List<object>();
        var totalDifferent = 0;
        foreach (var (name, width, text) in cases) {
            var bounds = new Rectangle(40, 30, width, 160);
            var timer = System.Diagnostics.Stopwatch.StartNew();
            using var croppedBackground = background.Clone(bounds, background.PixelFormat);
            using var input = name == "wrap"
                ? new NativeInlineText(parent, croppedBackground, bounds, text, backgroundOrigin: bounds.Location)
                : new NativeInlineText(parent, background, bounds, text);
            input.CreateControl();
            if (name.StartsWith("style-", StringComparison.Ordinal)) {
                var handle = input.Handle;
                input.Select(1, 3);
                input.SetTextStyle(name == "style-blue" ? 36 : 14, Color.ForestGreen);
                input.SetTextStyle(name == "style-blue" ? 36 : 14, name == "style-blue" ? Color.RoyalBlue : Color.Black);
                if (input.Font.Height <= 0) throw new Exception("Color change invalidated the text font");
                if (input.Handle != handle || input.SelectionStart != 1 || input.SelectionLength != 3 || input.Text != text)
                    throw new Exception("Text style replaced the input or changed the selection");
                if (input.FormattingRectangle.Left != 0 || input.FormattingRectangle.Right != input.ClientSize.Width)
                    throw new Exception("Changing text size shifted the text anchor");
                input.Select(0, 0);
            }
            var inputCreatedMs = timer.Elapsed.TotalMilliseconds;
            using var actual = new Bitmap(bounds.Width, bounds.Height);
            input.DrawToBitmap(actual, new Rectangle(Point.Empty, actual.Size));
            using var expected = RenderNative(input, background, bounds);
            var different = 0;
            for (var y = 0; y < actual.Height; y++) for (var x = 0; x < actual.Width; x++)
                if (actual.GetPixel(x, y).ToArgb() != expected.GetPixel(x, y).ToArgb()) different++;
            actual.Save(Path.Combine(output, name + "-input.png")); expected.Save(Path.Combine(output, name + "-output.png"));
            results.Add(new { name, differentPixels = different, inputCreatedMs, format = input.FormattingRectangle });
            totalDifferent += different;
        }
        var report = new { totalDifferent, results, scope = "Native EDIT DrawToBitmap versus rendering its actual visual lines; no user window or IME operation" };
        File.WriteAllText(Path.Combine(output, "report.json"), JsonSerializer.Serialize(report));
        Console.WriteLine(JsonSerializer.Serialize(report));
        return totalDifferent == 0 ? 0 : 1;
    }
}
