using System;
using System.Drawing;
using ShareX.HelpersLib;

namespace ShareX.ScreenCaptureLib
{
    // Prepare all image-sized allocations before committing the saved draft.
    // The form takes ownership only when the replacement is applied.
    public sealed class WinOneCanvasReplacement : IDisposable
    {
        internal Bitmap Image;
        internal Bitmap Background;
        public Size Size => Image?.Size ?? throw new ObjectDisposedException(nameof(WinOneCanvasReplacement));
        public static WinOneCanvasReplacement Prepare(Bitmap source) => new WinOneCanvasReplacement(source);
        internal WinOneCanvasReplacement(Bitmap source)
        {
            Image = new Bitmap(source);
            try {
                Background = new Bitmap(source.Width, source.Height);
                using var graphics = Graphics.FromImage(Background);
                var theme = ShareXResources.Theme;
                if (theme.CheckerSize > 0) {
                    using var checkers = ImageHelpers.DrawCheckers(source.Width, source.Height, theme.CheckerSize, theme.CheckerColor, theme.CheckerColor2);
                    graphics.DrawImageUnscaled(checkers, Point.Empty);
                } else graphics.Clear(theme.CheckerColor);
                graphics.DrawImageUnscaled(Image, Point.Empty);
            } catch { Dispose(); throw; }
        }
        public void Dispose() { Image?.Dispose(); Background?.Dispose(); Image = Background = null; }
    }

    public sealed partial class WinOneCaptureCanvas
    {
        public void ApplyReplacement(WinOneCanvasReplacement replacement)
        {
            if (!form.WinOnePresentation || form.IsDisposed) throw new InvalidOperationException("截图窗口不可用");
            var size = replacement.Size;
            form.ShapeManager.ResetWinOneForReplacement();
            form.ReplaceWinOneEditorImage(replacement);
            SetRegion(new Rectangle(Point.Empty, size));
            SelectTool("select");
            Changed?.Invoke(); DocumentChanged?.Invoke();
        }
    }

    public sealed partial class RegionCaptureForm
    {
        internal void ReplaceWinOneEditorImage(WinOneCanvasReplacement replacement)
        {
            var image = replacement.Image;
            var background = replacement.Background;
            if (image == null || background == null) throw new ObjectDisposedException(nameof(WinOneCanvasReplacement));
            // A toolbar scroll capture becomes a zoomable long image in this
            // same HWND. Mode changes only after acquisition has succeeded.
            Mode = RegionCaptureMode.Editor;
            ShapeManager.WindowCaptureMode = false;
            var previous = Canvas; var previousBackground = captureBackground;
            Canvas = image; captureBackground = background;
            replacement.Image = replacement.Background = null;
            backgroundHighlightBrush?.Dispose(); backgroundHighlightBrush = null;
            DimmedCanvas?.Dispose(); DimmedCanvas = null;
            CanvasRectangle = new RectangleF(CanvasRectangle.Location, image.Size);
            previous?.Dispose(); previousBackground?.Dispose();
            UpdateTitle(); CenterCanvas(); ZoomToFit(); Invalidate();
        }
    }

    internal partial class ShapeManager
    {
        internal void ResetWinOneForReplacement()
        {
            WinOneOrderedEffects=false;ResetWinOneComposition();
            IsCreating = IsMoving = IsResizing = false;
            ClearTools(); DeleteAllShapes();
            // History belongs to the previous image. Its owned bitmaps must be
            // released, and Undo must not revive a retired capture.
            history.Dispose(); ResetModifiers();
        }
    }
}
