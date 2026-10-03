using System.Text.Json;
using WinOne.Capture;

internal static class EditorFixture
{
    public static void NativeDraft(string directory, bool selecting = false)
    {
        // Match physical monitor geometry, but never read any desktop pixels.
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
        var bounds = SystemInformation.VirtualScreen;
        var image = new Bitmap(bounds.Width, bounds.Height);
        using (var graphics = Graphics.FromImage(image))
        using (var font = new Font("Segoe UI", 32))
        {
            graphics.Clear(Color.WhiteSmoke);
            for (var y = 0; y < image.Height; y += 100)
            for (var x = 0; x < image.Width; x += 100)
                graphics.DrawString($"{x}, {y}", font, Brushes.SteelBlue, x, y);
        }
        using var frame = CapturedFrame.Own(image, bounds);
        var work = Screen.PrimaryScreen!.WorkingArea;
        var selection = new Rectangle(work.X + 32, work.Y + 32, Math.Min(1600, work.Width - 64), Math.Min(900, work.Height - 64));
        var operations = new CaptureOperations(new SettingsStore(directory), directory);
        var settings = new CaptureSettings { OutputDirectory = directory, OpenEditor = true, CopyToClipboard = false };
        var draft = selecting ? operations.PreparePendingSelection(frame, "region", settings) : operations.PrepareSelectionEditor(frame, selection, settings);
        Console.WriteLine(JsonSerializer.Serialize(new { id = draft.Id, width = draft.Width, height = draft.Height }, SettingsStore.Json));
    }

    public static void Render(string directory)
    {
        Directory.CreateDirectory(directory);
        using var source = new Bitmap(3200, 1600);
        using (var graphics = Graphics.FromImage(source))
        using (var title = new Font("Segoe UI", 96))
        using (var body = new Font("Segoe UI", 56))
        using (var accent = new SolidBrush(Color.FromArgb(40, 95, 145)))
        using (var pale = new SolidBrush(Color.FromArgb(220, 230, 240)))
        {
            graphics.Clear(Color.White);
            graphics.DrawString("WinOne", title, accent, 180, 120);
            graphics.DrawString("Capture editor • Native preview", body, Brushes.DimGray, 180, 320);
            for (var y = 580; y < 1100; y += 20)
            for (var x = 180; x < 3000; x += 20)
                graphics.FillRectangle(((x + y) / 20) % 2 == 0 ? pale : accent, x, y, 20, 20);
            graphics.DrawString("Mosaic                 Blur                 Original", body, Brushes.Black, 180, 1180);
        }
        var previews = new CapturePreviews(directory);
        previews.Write(source, new string('a', 32));
        PngFiles.Write(source, Path.Combine(directory, "drafts", new string('a', 32) + ".png"));
        using var longSource = new Bitmap(720, 18000);
        using (var graphics = Graphics.FromImage(longSource))
        using (var font = new Font("Segoe UI", 24))
        {
            graphics.Clear(Color.White);
            for (var y = 0; y < longSource.Height; y += 100)
                graphics.DrawString($"WinOne · Row {y}", font, Brushes.SteelBlue, 24, y + 20);
        }
        PngFiles.Write(longSource, Path.Combine(directory, "drafts", new string('e', 32) + ".png"));
        var path = Path.Combine(directory, "document.json");
        if (!File.Exists(path)) return;
        using var input = File.OpenRead(path);
        if (input.Length > 768 * 1024) throw new InvalidDataException("Fixture document too large");
        var document = JsonSerializer.Deserialize<AnnotationDocument>(input, SettingsStore.Json) ?? throw new InvalidDataException("Missing fixture document");
        using var output = AnnotationRenderer.Render(document.Width == 720 && document.Height == 18000 ? longSource : source, document);
        PngFiles.Write(output, Path.Combine(directory, "previews", new string('b', 32) + ".png"));
    }
}
