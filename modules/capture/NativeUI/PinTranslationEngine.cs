using System.Drawing.Imaging;
using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;

namespace WinOne.Capture.NativeUI;

internal sealed record PinTranslationResult(Bitmap Picture, string Text);

// The ABI owns the original pixels, paragraphs and output text until this request ends.
internal sealed class PinPageHandle : SafeHandleZeroOrMinusOneIsInvalid
{
    private PinPageHandle() : base(true) { }
    protected override bool ReleaseHandle() { PinTranslationEngine.pin_destroy(handle); return true; }
}

internal static class PinTranslationEngine
{
    private const string Library = "WinOne.PinTranslation.dll";
    [StructLayout(LayoutKind.Sequential)]
    private record struct Box(int X, int Y, int Width, int Height)
    {
        internal static Box From(int[] v) => v.Length == 4 ? new(v[0], v[1], v[2], v[3]) : throw new InvalidDataException("ocrFailed");
        internal Rectangle Rect => new(X, Y, Width, Height);
    }

    [DllImport(Library)] private static extern PinPageHandle pin_create(IntPtr pixels, int width, int height, int stride);
    [DllImport(Library)] internal static extern void pin_destroy(IntPtr page);
    [DllImport(Library)] private static extern int pin_obstacle(PinPageHandle page, Box box);
    [DllImport(Library)] private static extern int pin_add(PinPageHandle page, [MarshalAs(UnmanagedType.LPUTF8Str)] string text, Box box, Box[] chars, int count, int region, int row);
    [DllImport(Library)] private static extern int pin_prepare(PinPageHandle page);
    [DllImport(Library)] private static extern IntPtr pin_text(PinPageHandle page, int index);
    [DllImport(Library)] private static extern int pin_translated(PinPageHandle page, int index, [MarshalAs(UnmanagedType.LPUTF8Str)] string text);
    [DllImport(Library)] private static extern int pin_render(PinPageHandle page, [MarshalAs(UnmanagedType.LPUTF8Str)] string target, IntPtr pixels, int stride);
    [DllImport(Library)] private static extern IntPtr pin_combined(PinPageHandle page);

    internal static async Task<PinTranslationResult> Translate(Bitmap original, PinOcrLayout layout, Rectangle? selection,
        Func<string, CancellationToken, Task<string>> resolveTarget,
        Func<string, string, CancellationToken, Task<string>> translate, CancellationToken cancel,
        IProgress<TranslationProgress>? progress = null)
    {
        var bounds = new Rectangle(Point.Empty, original.Size);
        var input = original.LockBits(bounds, ImageLockMode.ReadOnly, PixelFormat.Format32bppArgb);
        PinPageHandle handle;
        try { handle = pin_create(input.Scan0, original.Width, original.Height, input.Stride); }
        finally { original.UnlockBits(input); }
        using var page = handle;
        if (page.IsInvalid) throw new InvalidOperationException("pinRenderFailed");
        foreach (var line in layout.Lines)
        {
            var box = Box.From(line.Box);
            if (pin_obstacle(page, box) != 1) throw new InvalidOperationException("pinRenderFailed");
            var text = line.Text;
            var glyphs = line.Glyphs;
            if (selection is { } region)
            {
                if (!region.IntersectsWith(box.Rect)) continue;
                if (!region.Contains(box.Rect) && glyphs.Length > 0)
                {
                    glyphs = glyphs.Where(g => {
                        var r = Box.From(g.Box);
                        return region.Contains(r.X + r.Width / 2, r.Y + r.Height / 2);
                    }).ToArray();
                    if (glyphs.Length == 0) continue;
                    // Keep the original spacing when a range falls inside one OCR line.
                    text = SelectedText(line, glyphs);
                    var rect = glyphs.Select(g => Box.From(g.Box).Rect).Aggregate(Rectangle.Union);
                    box = new(rect.X, rect.Y, rect.Width, rect.Height);
                }
            }
            var chars = glyphs.Select(g => Box.From(g.Box)).ToArray();
            if (pin_add(page, text, box, chars, chars.Length, 0, -1) != 1)
                throw new InvalidOperationException("pinRenderFailed");
        }
        var count = pin_prepare(page);
        if (count < 1) throw new InvalidOperationException(count == 0 ? "ocrEmpty" : "pinRenderFailed");
        var paragraphs = Enumerable.Range(0, count).Select(i => Marshal.PtrToStringUTF8(pin_text(page, i))
            ?? throw new InvalidDataException("pinRenderFailed")).ToArray();
        // OCR is done; paragraph translation spans 20%..90%, rendering the rest.
        progress?.Report(new(0.2, "translate", 0, count));
        var target = await resolveTarget(string.Join("\n", paragraphs), cancel).ConfigureAwait(false);
        for (int i = 0; i < count; i++)
        {
            cancel.ThrowIfCancellationRequested();
            var result = await translate(paragraphs[i], target, cancel).ConfigureAwait(false);
            if (string.IsNullOrWhiteSpace(result) || pin_translated(page, i, result) != 1) throw new InvalidDataException("translationFailed");
            progress?.Report(new(0.2 + 0.7 * (i + 1) / count, "translate", i + 1, count));
        }
        cancel.ThrowIfCancellationRequested();
        progress?.Report(new(0.92, "render", count, count));
        var output = new Bitmap(original.Width, original.Height, PixelFormat.Format32bppArgb);
        try
        {
            var pixels = output.LockBits(bounds, ImageLockMode.WriteOnly, PixelFormat.Format32bppArgb);
            int rendered;
            try { rendered = pin_render(page, target, pixels.Scan0, pixels.Stride); }
            finally { output.UnlockBits(pixels); }
            if (rendered != 1) throw new InvalidOperationException(rendered == 0 ? "pinUnchanged" : "pinRenderFailed");
            return new(output, Marshal.PtrToStringUTF8(pin_combined(page)) ?? "");
        }
        catch { output.Dispose(); throw; }
    }

    private static string SelectedText(PinOcrLine line, PinGlyph[] selected)
    {
        int next = 0, first = -1, last = -1;
        var wanted = selected.ToHashSet();
        foreach (var glyph in line.Glyphs)
        {
            var start = line.Text.IndexOf(glyph.Text, next, StringComparison.Ordinal);
            if (start < 0) return string.Concat(selected.Select(g => g.Text));
            next = start + glyph.Text.Length;
            if (!wanted.Contains(glyph)) continue;
            if (first < 0) first = start;
            last = next;
        }
        return line.Text[first..last];
    }
}
