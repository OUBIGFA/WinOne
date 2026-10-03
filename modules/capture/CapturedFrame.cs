namespace WinOne.Capture;

// A single owned desktop image in physical pixel coordinates. Rectangle right
// and bottom edges are exclusive; negative virtual-screen origins are valid.
internal sealed class CapturedFrame : IDisposable
{
    private readonly object gate = new();
    private Bitmap? image;
    public Rectangle Bounds { get; }

    private CapturedFrame(Bitmap image, Rectangle bounds) { this.image = image; Bounds = bounds; }

    // Ownership transfers only after validation succeeds. The producer must not
    // mutate or dispose the bitmap after giving it to this frame.
    public static CapturedFrame Own(Bitmap image, Rectangle bounds)
    {
        ArgumentNullException.ThrowIfNull(image);
        if (bounds.Width <= 0 || bounds.Height <= 0 ||
            (long)bounds.X + bounds.Width > int.MaxValue ||
            (long)bounds.Y + bounds.Height > int.MaxValue || image.Size != bounds.Size)
            throw new ArgumentException("图像尺寸与桌面范围不一致", nameof(bounds));
        return new CapturedFrame(image, bounds);
    }

    public Bitmap Crop(Rectangle selection)
    {
        lock (gate)
        {
            var source = image ?? throw new ObjectDisposedException(nameof(CapturedFrame));
            // Use wide arithmetic before converting desktop coordinates to image
            // offsets, so malformed rectangles cannot wrap into a valid crop.
            if (selection.Width <= 0 || selection.Height <= 0 || selection.X < Bounds.X || selection.Y < Bounds.Y ||
                (long)selection.X + selection.Width > (long)Bounds.X + Bounds.Width ||
                (long)selection.Y + selection.Height > (long)Bounds.Y + Bounds.Height)
                throw new ArgumentOutOfRangeException(nameof(selection), "选区超出截图范围");
            var local = new Rectangle(selection.X - Bounds.X, selection.Y - Bounds.Y, selection.Width, selection.Height);
            return source.Clone(local, source.PixelFormat);
        }
    }

    // Native selection paints the owned buffer directly. No image handle is
    // exposed to the view, and disposal cannot race a paint operation.
    public void DrawUnscaled(Graphics graphics)
    {
        lock (gate)
        {
            var source = image ?? throw new ObjectDisposedException(nameof(CapturedFrame));
            graphics.DrawImageUnscaled(source, Point.Empty);
        }
    }

    // Borrow the source under the same lifetime lock; only a bounded thumbnail
    // leaves this frame, never a second full-desktop allocation.
    public Bitmap CreatePreview()
    {
        lock (gate) { return CapturePreviews.CreateThumbnail(image ?? throw new ObjectDisposedException(nameof(CapturedFrame))); }
    }

    internal T WithImage<T>(Func<Bitmap, T> action)
    {
        lock (gate) return action(image ?? throw new ObjectDisposedException(nameof(CapturedFrame)));
    }

    // Fullscreen export transfers the original allocation instead of making an
    // unnecessary second full-resolution image. Cropped outputs remain separate.
    public Bitmap Detach()
    {
        lock (gate)
        {
            var result = image ?? throw new ObjectDisposedException(nameof(CapturedFrame));
            image = null;
            return result;
        }
    }

    public void Dispose()
    {
        lock (gate) { image?.Dispose(); image = null; }
    }
}
