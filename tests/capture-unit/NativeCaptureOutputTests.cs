using WinOne.Capture;

internal static class NativeCaptureOutputTests
{
    public static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        var store = new SettingsStore(directory);
        var outputDirectory = Path.Combine(directory, "images");
        store.Save(store.Snapshot() with { OutputDirectory = outputDirectory, FileNameTemplate = "NativeOutput-{sequence}", CopyToClipboard = false });
        var copies = 0; string? selectedPath = null;
        var operations = new CaptureOperations(store, directory, _ => copies++, (_, _) => selectedPath);
        var output = operations.CreateNativeOutput(CancellationToken.None);
        store.Save(store.Snapshot() with { OutputDirectory = Path.Combine(directory, "next-session"), FileNameTemplate = "Changed", CopyToClipboard = true });
        using var image = new Bitmap(12, 8);
        using (var graphics = Graphics.FromImage(image)) graphics.Clear(Color.Coral);
        var first = output.Save(image); var second = output.Save(image);
        if (Path.GetFileName(first.Path) != "NativeOutput-01.png" || Path.GetFileName(second.Path) != "NativeOutput-02.png" || Path.GetDirectoryName(first.Path) != outputDirectory)
            throw new Exception("Native output lost its settings snapshot or unique naming");
        using (var saved = new Bitmap(first.Path))
            if (saved.Size != image.Size || saved.GetPixel(0, 0).ToArgb() != Color.Coral.ToArgb()) throw new Exception("Native save changed pixels");
        if (copies != 0) throw new Exception("Active session picked up later copy preferences");
        output.Confirm(image);
        if (copies != 1 || Directory.GetFiles(outputDirectory).Length != 2) throw new Exception("Confirm implicitly saved an image");
        try { output.SaveAs(image); throw new Exception("Cancelled save-as succeeded"); }
        catch (OperationCanceledException) { }
        selectedPath = Path.Combine(directory, "chosen.png");
        var saveAs = operations.CreateNativeOutput(CancellationToken.None).SaveAs(image);
        if (saveAs.Path != selectedPath || !File.Exists(selectedPath) || copies != 1) throw new Exception("Save-as repeated copy or ignored the chosen path");
        var failed = new CaptureOperations(store, Path.Combine(directory, "failure"), _ => throw new IOException("copy unavailable"));
        try { failed.CreateNativeOutput(CancellationToken.None).Confirm(image); throw new Exception("Copy failure was ignored"); }
        catch (IOException error) when (error.Message == "copy unavailable") { }
        using var stopped = new CancellationTokenSource(); stopped.Cancel();
        try { operations.CreateNativeOutput(stopped.Token).Save(image); throw new Exception("Stopped session saved an image"); }
        catch (OperationCanceledException) { }
        SaveCopyTests.Run(Path.Combine(directory, "legacy-save-copy"));
        Console.WriteLine("PASS: native output settings snapshot, naming, pixels, confirm, save-as and failure propagation");
    }
}
