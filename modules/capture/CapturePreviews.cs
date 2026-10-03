using System.Drawing.Drawing2D;
using System.Drawing.Imaging;
using System.Text.Json;

namespace WinOne.Capture;

internal record CaptureEditPreview(string Id, string SourceId, int Width, int Height);

internal sealed class CapturePreviews(string directory)
{
    public CaptureEditPreview WriteEdit(Bitmap image, string sourceId)
    {
        if (!Guid.TryParseExact(sourceId, "N", out _)) throw new InvalidDataException("原图标识无效");
        var metadata = Path.Combine(directory, "edit-preview.json");
        CaptureEditPreview? previous = null;
        if (File.Exists(metadata))
        {
            using var stream = File.OpenRead(metadata);
            if (stream.Length > 16 * 1024) throw new InvalidDataException("编辑预览记录过大");
            previous = JsonSerializer.Deserialize<CaptureEditPreview>(stream, SettingsStore.Json);
        }
        var preview = new CaptureEditPreview(Guid.NewGuid().ToString("N"), sourceId, image.Width, image.Height);
        // Effect previews replace the canvas raster, so downsampling here would
        // blur untouched pixels as soon as a mosaic or blur mark is added.
        PngFiles.Write(image, FilePath(preview.Id));
        try { SettingsStore.AtomicWrite(metadata, JsonSerializer.SerializeToUtf8Bytes(preview, SettingsStore.Json)); }
        catch
        {
            Remove(preview.Id);
            throw;
        }
        // Commit the new identity before retiring the previous file. Readers
        // can only fetch the one bound to current metadata and its source ID.
        if (previous is not null) Remove(previous.Id);
        return preview;
    }
    private string FilePath(string id)
    {
        if (!Guid.TryParseExact(id, "N", out _)) throw new InvalidDataException("预览标识无效");
        return Path.Combine(directory, "previews", id + ".png");
    }
    public void Write(Bitmap image, string id)
    {
        if (Math.Max(image.Width, image.Height) <= 1600) { PngFiles.Write(image, FilePath(id)); return; }
        using var preview = CreateThumbnail(image);
        PngFiles.Write(preview, FilePath(id));
    }
    internal static Bitmap CreateThumbnail(Bitmap image)
    {
        const int maximum = 1600;
        var scale = Math.Min(1.0, maximum / (double)Math.Max(image.Width, image.Height));
        var size = new Size(Math.Max(1, (int)Math.Round(image.Width * scale)), Math.Max(1, (int)Math.Round(image.Height * scale)));
        var preview = new Bitmap(size.Width, size.Height, PixelFormat.Format32bppArgb);
        try
        {
        using (var graphics = Graphics.FromImage(preview))
        using (var attributes = new ImageAttributes())
        {
            graphics.CompositingMode = CompositingMode.SourceCopy;
            graphics.InterpolationMode = InterpolationMode.HighQualityBicubic;
            graphics.PixelOffsetMode = PixelOffsetMode.HighQuality;
            attributes.SetWrapMode(WrapMode.TileFlipXY);
            graphics.DrawImage(image, new Rectangle(Point.Empty, size), 0, 0, image.Width, image.Height, GraphicsUnit.Pixel, attributes);
        }
            return preview;
        }
        catch { preview.Dispose(); throw; }
    }
    public void Remove(string id) { if (Guid.TryParseExact(id, "N", out _)) File.Delete(FilePath(id)); }
}
