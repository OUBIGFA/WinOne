using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

internal sealed record NativeCaptureSnapshot(WinOneCaptureDocument Document, NativeTextInputState? Text = null);

// Keep unfinished input separate from committed layers. Autosave must neither
// commit an IME composition nor add an undo step merely to preserve text.
internal sealed record NativeTextInputState(Guid? Target, string Value, int X, int Y, int Width, int Height,
    int MaxHeight, float FontPixels, int ColorArgb, int SelectionStart, int SelectionLength)
{
    [System.Text.Json.Serialization.JsonIgnore]
    public Rectangle Bounds => new(X, Y, Width, Height);
    public void Validate(WinOneCaptureDocument document)
    {
        if (Value is null || Value.Length > 4096 || document.Selection is not { } selection ||
            X < selection.X || Y < selection.Y || Width < 1 || Height < 1 || MaxHeight < Height ||
            (long)X + Width > selection.Right || (long)Y + MaxHeight > selection.Bottom ||
            !float.IsFinite(FontPixels) || FontPixels < 6 || FontPixels > 256 || Color.FromArgb(ColorArgb).A != 255 ||
            SelectionStart < 0 || SelectionLength < 0 || (long)SelectionStart + SelectionLength > Value.Length ||
            Target is { } target && !document.Layers.Any(layer => layer.Kind == "text" && layer.TextId == target))
            throw new InvalidDataException("截图文字草稿无效");
    }
}
