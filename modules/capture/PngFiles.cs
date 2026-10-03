using System.Drawing.Imaging;

namespace WinOne.Capture;

internal static class PngFiles
{
    // Resolve the encoder once under CLR type initialization. Image.Save's
    // format lookup can otherwise race during simultaneous first-time saves.
    private static readonly ImageCodecInfo pngEncoder = ImageCodecInfo.GetImageEncoders().Single(codec => codec.FormatID == ImageFormat.Png.Guid);
    public static string SuggestedName(DateTime time, int sequence = 1, string? template = null) => CaptureFileName.Render(template, time, sequence);

    // Encode once, then claim the final name atomically. A check-before-write
    // would allow simultaneous captures to overwrite one another.
    public static string WriteUnique(Image image, string directory, DateTime time, string? template = null)
    {
        if (!Path.IsPathFullyQualified(directory)) throw new InvalidOperationException("保存目录必须为绝对路径");
        Directory.CreateDirectory(directory);
        var temporary = Path.Combine(directory, ".winone-" + Guid.NewGuid().ToString("N") + ".tmp");
        try
        {
            using (var file = new FileStream(temporary, FileMode.CreateNew, FileAccess.Write, FileShare.None, 4096, FileOptions.WriteThrough))
            { image.Save(file, pngEncoder, null); file.Flush(true); }
            for (var sequence = 1; sequence < int.MaxValue; sequence++)
            {
                var path = Path.Combine(directory, SuggestedName(time, sequence, template));
                try { File.Move(temporary, path, false); return path; }
                catch (IOException) when (File.Exists(path) || Directory.Exists(path)) { }
            }
            throw new IOException("截图文件序号已用尽");
        }
        finally { if (File.Exists(temporary)) File.Delete(temporary); }
    }

    // Encode directly to the temporary file; a full PNG MemoryStream plus
    // ToArray would unnecessarily duplicate large captures in memory.
    public static void Write(Image image, string path)
    {
        if (!Path.IsPathFullyQualified(path) || !path.EndsWith(".png", StringComparison.OrdinalIgnoreCase))
            throw new InvalidOperationException("请选择 PNG 文件的完整保存路径");
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        var temporary = path + "." + Guid.NewGuid().ToString("N") + ".tmp";
        try
        {
            using (var file = new FileStream(temporary, FileMode.CreateNew, FileAccess.Write, FileShare.None, 4096, FileOptions.WriteThrough))
            { image.Save(file, pngEncoder, null); file.Flush(true); }
            File.Move(temporary, path, true);
        }
        finally { if (File.Exists(temporary)) File.Delete(temporary); }
    }
}
