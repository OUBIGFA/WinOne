using WinOne.Capture;

internal static class PngFileTests
{
    public static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        var time = new DateTime(2026, 10, 1, 9, 8, 7);
        using var fixtures = System.Text.Json.JsonDocument.Parse(File.ReadAllText(Path.Combine(AppContext.BaseDirectory, "capture-filenames.json")));
        foreach (var fixture in fixtures.RootElement.EnumerateArray())
        {
            var template = fixture.GetProperty("template").GetString();
            var sequence = fixture.GetProperty("sequence").GetInt32();
            var expected = fixture.GetProperty("expected").GetString();
            string? actual = null;
            try { actual = CaptureFileName.Render(template, time, sequence); }
            catch (InvalidDataException) when (expected is null) { }
            if (actual != expected) throw new Exception("filename template contract mismatch: " + template);
        }
        if (PngFiles.SuggestedName(time) != "WinOne-20261001090807-OK.png") throw new Exception("default capture name changed");
        using (var freshImage = new Bitmap(2, 2))
        {
            var freshDirectory = Path.Combine(directory, "fresh");
            var names = Enumerable.Range(0, 3).Select(_ => Path.GetFileName(PngFiles.WriteUnique(freshImage, freshDirectory, time))).ToArray();
            if (!names.SequenceEqual(new[] { "WinOne-20261001090807-OK.png", "WinOne-20261001090807-OK-01.png", "WinOne-20261001090807-OK-02.png" })) throw new Exception("default collision suffix was not added only after the first save");
        }
        var first = Path.Combine(directory, PngFiles.SuggestedName(time));
        File.WriteAllText(first, "existing user file");
        Directory.CreateDirectory(Path.Combine(directory, PngFiles.SuggestedName(time, 2)));
        var tasks = Enumerable.Range(0, 8).Select(_ => Task.Run(() =>
        {
            using var image = new Bitmap(4, 3);
            image.SetPixel(1, 1, Color.Coral);
            return PngFiles.WriteUnique(image, directory, time);
        })).ToArray();
        Task.WaitAll(tasks);
        var paths = tasks.Select(task => task.Result).ToArray();
        if (paths.Distinct(StringComparer.OrdinalIgnoreCase).Count() != 8) throw new Exception("concurrent captures reused a filename");
        if (File.ReadAllText(first) != "existing user file") throw new Exception("capture overwrote an existing file");
        for (var sequence = 3; sequence <= 10; sequence++)
        {
            var path = Path.Combine(directory, PngFiles.SuggestedName(time, sequence));
            using var image = new Bitmap(path);
            if (image.GetPixel(1, 1).ToArgb() != Color.Coral.ToArgb()) throw new Exception("unique capture output was incomplete");
        }
        if (Directory.EnumerateFiles(directory, "*.tmp").Any()) throw new Exception("capture temporary files were retained");
        using var custom = new Bitmap(2, 2);
        var customFirst = PngFiles.WriteUnique(custom, directory, time, "报告");
        var customSecond = PngFiles.WriteUnique(custom, directory, time, "报告");
        if (Path.GetFileName(customFirst) != "报告.png" || Path.GetFileName(customSecond) != "报告-02.png") throw new Exception("literal template overwrote a capture");
    }
}
