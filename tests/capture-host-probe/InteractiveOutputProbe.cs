using System.Text.Json;
using WinOne.Capture;

// Explicit interactive check only. It writes a generated image to the real
// clipboard and opens the product Save As picker, never a user's document.
internal static class InteractiveOutputProbe
{
    public static int Run(string directory)
    {
        Directory.CreateDirectory(directory);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        // Like the full-screen capture canvas, this owner encloses its modal
        // picker so window-scoped automation can reach the owned dialog.
        using var owner = new Form { Text = "WinOne 输出验证", Width = 1400, Height = 950, StartPosition = FormStartPosition.CenterScreen };
        owner.Controls.Add(new Label { Dock = DockStyle.Fill, Text = "验证剪贴板与另存为", TextAlign = ContentAlignment.MiddleCenter });
        var code = 0;
        owner.Shown += (_, _) => owner.BeginInvoke((Action)(() => {
            try { Verify(directory); }
            catch (Exception error) { code = 1; File.WriteAllText(Path.Combine(directory, "error.txt"), error.ToString()); }
            finally { owner.Close(); }
        }));
        Application.Run(owner);
        return code;
    }
    private static void Verify(string directory)
    {
        void Stage(string value) => File.WriteAllText(Path.Combine(directory, "stage.txt"), value);
        var settings = new SettingsStore(directory);
        settings.Save(settings.Snapshot() with { OutputDirectory = directory, CopyToClipboard = false, AskWhereToSave = false });
        var operations = new CaptureOperations(settings, directory);
        var output = operations.CreateNativeOutput(CancellationToken.None, "zh-CN");
        using var image = new Bitmap(96, 64);
        using (var g = Graphics.FromImage(image)) {
            g.Clear(Color.White); g.FillRectangle(Brushes.RoyalBlue, 0, 0, 48, 32);
            g.FillRectangle(Brushes.Coral, 48, 32, 48, 32);
        }
        void Same(Bitmap actual) {
            if (actual.Size != image.Size) throw new Exception("Image dimensions changed");
            for (var y = 0; y < image.Height; y++) for (var x = 0; x < image.Width; x++)
                if (actual.GetPixel(x, y) != image.GetPixel(x, y)) throw new Exception("Image pixels changed");
        }
        Stage("copy"); output.Confirm(image);
        Stage("read clipboard");
        using (var copied = Clipboard.GetImage() ?? throw new Exception("Clipboard image missing"))
        using (var bitmap = new Bitmap(copied)) Same(bitmap);
        Stage("cancel picker"); var cancelled = false;
        try { output.SaveAs(image); }
        catch (OperationCanceledException) { cancelled = true; }
        if (!cancelled || Directory.GetFiles(directory, "*.png").Length != 0)
            throw new Exception("Cancel did not leave output untouched");
        Stage("save picker"); var saved = output.SaveAs(image);
        if (!Path.GetFullPath(saved.Path).StartsWith(Path.GetFullPath(directory) + Path.DirectorySeparatorChar, StringComparison.OrdinalIgnoreCase))
            throw new Exception("Test output escaped its directory");
        using (var result = new Bitmap(saved.Path)) Same(result);
        File.WriteAllText(Path.Combine(directory, "verification.json"), JsonSerializer.Serialize(new {
            clipboardPixelsIdentical = true, pickerCancelled = cancelled, savedPixelsIdentical = true,
            saved.Width, saved.Height, file = Path.GetFileName(saved.Path)
        }));
    }
}
