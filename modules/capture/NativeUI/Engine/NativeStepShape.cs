using System;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Globalization;
using System.Linq;

namespace ShareX.ScreenCaptureLib
{
    // Pixel metrics of a numbered step. Everything derives from the label font
    // size, so badge, tail and label keep the same proportions at every size.
    public sealed record WinOneStepMetrics(int Badge, int Gap, int Tail, int PadX, int PadY, int Radius, int Line, int MinText)
    {
        public static WinOneStepMetrics For(float fontPixels)
        {
            int Scaled(float factor, int minimum) => Math.Max(minimum, (int)Math.Round(fontPixels * factor));
            // The inline EDIT is sized in Font.Height rows; use the same line box
            // so the badge centres on the first label line.
            using var font = new Font("Segoe UI", fontPixels, FontStyle.Regular, GraphicsUnit.Pixel);
            // An even badge keeps its centre on a whole pixel.
            return new WinOneStepMetrics(2 * Scaled(0.625f, 4), Scaled(0.2f, 2), Scaled(0.35f, 3), Scaled(0.5f, 4), Scaled(0.3f, 2), Scaled(0.2f, 2), font.Height, Scaled(2.5f, 12));
        }

        // Label text origin, relative to the badge centre.
        public Point TextOffset => new Point(Badge / 2 + Gap + Tail + PadX, -(Line / 2));

        // The nearest badge centre whose badge and smallest label stay inside
        // the crop. Null when the crop cannot hold a step of this size.
        public Point? Fit(Rectangle area, Point point)
        {
            var left = area.Left + Badge / 2; var right = area.Right - (TextOffset.X + MinText + PadX);
            var top = area.Top + Line / 2 + PadY; var bottom = area.Bottom - (Line - Line / 2 + PadY);
            if (right < left || bottom < top) return null;
            return new Point(Math.Clamp(point.X, left, right), Math.Clamp(point.Y, top, bottom));
        }
    }

    public sealed record WinOneStepValue(Guid Id, int Number, string Text, Rectangle TextBounds, float FontPixels, int ColorArgb);

    // A numbered badge with an optional label bubble. The engine's own step
    // ordering assigns Number, so deleting or undoing a step renumbers the rest.
    // Rectangle is the badge alone, or badge plus bubble once a label exists;
    // the bubble is therefore present exactly when Rectangle is wider than the badge.
    public sealed class WinOneStepShape : StepDrawingShape
    {
        public static readonly Color LabelFill = Color.FromArgb(0x5c, 0x5c, 0x5c);
        public Guid Id { get; set; } = Guid.NewGuid();
        public string Text { get; set; } = "";
        public float FontPixels { get; set; } = 16;
        public WinOneTextLine[] NativeLines { get; set; }
        // Label size while the inline editor owns the text. The committed
        // Rectangle stays untouched, so cancelling an edit restores nothing.
        public SizeF? EditingSize { get; set; }

        private static bool HasBubble(RectangleF bounds, WinOneStepMetrics m) => bounds.Width > m.Badge + 0.5f;
        private PointF Center(WinOneStepMetrics m) => new PointF(Rectangle.X + m.Badge / 2,
            Rectangle.Y + (HasBubble(Rectangle, m) ? m.PadY + m.Line / 2 : m.Badge / 2));
        private static RectangleF Bounds(PointF center, SizeF? text, WinOneStepMetrics m)
        {
            var badge = new RectangleF(center.X - m.Badge / 2, center.Y - m.Badge / 2, m.Badge, m.Badge);
            if (text is not { } size) return badge;
            // The first label line shares the badge's centre; more lines grow downwards.
            var top = center.Y + m.TextOffset.Y - m.PadY;
            return RectangleF.FromLTRB(badge.Left, top, center.X + m.TextOffset.X + size.Width + m.PadX, top + Math.Max(size.Height, m.Line) + 2 * m.PadY);
        }
        private static RectangleF TextArea(RectangleF bounds, WinOneStepMetrics m) => RectangleF.FromLTRB(
            bounds.Left + m.Badge / 2 + m.TextOffset.X, bounds.Top + m.PadY, bounds.Right - m.PadX, bounds.Bottom - m.PadY);

        internal void Place(PointF center) => Rectangle = Bounds(center, null, WinOneStepMetrics.For(FontPixels));
        internal RectangleF BoundsFor(SizeF? text) { var m = WinOneStepMetrics.For(FontPixels); return Bounds(Center(m), text, m); }
        internal RectangleF DrawBounds => EditingSize is { } size ? BoundsFor(size) : Rectangle;
        // Where the label text is, or where the editor opens for a new label.
        internal RectangleF LabelArea
        {
            get {
                var m = WinOneStepMetrics.For(FontPixels);
                return HasBubble(Rectangle, m) ? TextArea(Rectangle, m) : TextArea(Bounds(Center(m), new SizeF(m.MinText, m.Line), m), m);
            }
        }

        public override void OnConfigLoad() { base.OnConfigLoad(); BorderSize = 0; Shadow = false; }
        // The step keeps its own style; it never becomes the engine's step preset.
        public override void OnConfigSave() { }
        public override void OnCreating() => throw new InvalidOperationException("Use the step tool");
        // No resize handles or draggable tail: the label defines the size.
        public override void OnNodeVisible() { }
        public override void OnNodeUpdate() { }
        // Hover and hit-testing use the whole badge-and-label box, not the base ellipse.
        public override void OnShapePathRequested(GraphicsPath path, RectangleF bounds) => path.AddRectangle(bounds);
        // The base step also moves its tail handle, which needs a UI manager.
        public override void Move(float x, float y) => Rectangle = new RectangleF(Rectangle.X + x, Rectangle.Y + y, Rectangle.Width, Rectangle.Height);

        public override void OnDraw(Graphics graphics)
        {
            var m = WinOneStepMetrics.For(FontPixels);
            var bounds = DrawBounds; var center = Center(m);
            var state = graphics.Save();
            try {
                graphics.SmoothingMode = SmoothingMode.AntiAlias; graphics.PixelOffsetMode = PixelOffsetMode.Half;
                if (HasBubble(bounds, m)) {
                    using var path = BubblePath(RectangleF.FromLTRB(bounds.Left + m.Badge + m.Gap + m.Tail, bounds.Top, bounds.Right, bounds.Bottom), center.Y, m);
                    using var fill = new SolidBrush(LabelFill);
                    graphics.FillPath(fill, path);
                }
                using (var badge = new SolidBrush(Color.FromArgb(255, FillColor)))
                    graphics.FillEllipse(badge, center.X - m.Badge / 2, center.Y - m.Badge / 2, m.Badge, m.Badge);
                DrawDigits(graphics, center, m);
            } finally { graphics.Restore(state); }
            if (EditingSize is null && NativeLines != null && HasBubble(bounds, m))
                NativeDrawingSurface.DrawText(graphics, NativeLines, System.Drawing.Rectangle.Round(TextArea(bounds, m)), FontPixels, Color.White);
            // The step has no handles. While Move has it selected, keep the
            // engine's hover outline on it so Delete has a visible target.
            if (Manager is { IsRenderingOutput: false, NodesVisible: true } manager && manager.CurrentShape == this) {
                using var outline = new Pen(SystemColors.Highlight);
                graphics.DrawRectangle(outline, bounds.X, bounds.Y, bounds.Width - 1, bounds.Height - 1);
            }
        }

        private static GraphicsPath BubblePath(RectangleF box, float tailY, WinOneStepMetrics m)
        {
            // One winding path with both figures clockwise. The tail overlaps
            // the box by a pixel, so antialiasing leaves no seam between them.
            var path = new GraphicsPath(FillMode.Winding);
            var corner = Math.Min(2f * m.Radius, Math.Min(box.Width, box.Height));
            path.AddArc(box.Left, box.Top, corner, corner, 180, 90);
            path.AddArc(box.Right - corner, box.Top, corner, corner, 270, 90);
            path.AddArc(box.Right - corner, box.Bottom - corner, corner, corner, 0, 90);
            path.AddArc(box.Left, box.Bottom - corner, corner, corner, 90, 90);
            path.CloseFigure();
            path.AddPolygon(new[] { new PointF(box.Left + 1, tailY - m.Tail), new PointF(box.Left + 1, tailY + m.Tail), new PointF(box.Left - m.Tail, tailY) });
            return path;
        }

        private void DrawDigits(Graphics graphics, PointF center, WinOneStepMetrics m)
        {
            // Outline text centres on its ink box, so every number sits in the
            // middle of the badge. Three digits shrink to fit instead of
            // widening the badge, which would move the label on renumbering.
            using var font = new Font("Segoe UI Semibold", m.Badge * 0.72f, FontStyle.Regular, GraphicsUnit.Pixel);
            using var path = new GraphicsPath();
            path.AddString(Number.ToString(CultureInfo.InvariantCulture), font.FontFamily, (int)font.Style, font.Size, PointF.Empty, StringFormat.GenericTypographic);
            var ink = path.GetBounds();
            if (ink.Width <= 0 || ink.Height <= 0) return;
            var scale = Math.Min(1f, m.Badge * 0.8f / ink.Width);
            using var matrix = new Matrix();
            matrix.Translate(center.X, center.Y); matrix.Scale(scale, scale); matrix.Translate(-(ink.X + ink.Width / 2), -(ink.Y + ink.Height / 2));
            path.Transform(matrix);
            // White digits, except on a light badge color where they would vanish.
            var light = (FillColor.R * 0.299f + FillColor.G * 0.587f + FillColor.B * 0.114f) / 255 > 0.6f;
            using var brush = new SolidBrush(light ? Color.FromArgb(0x17, 0x17, 0x17) : Color.White);
            graphics.FillPath(brush, path);
        }
    }

    public sealed partial class WinOneCaptureCanvas
    {
        public int StepCount => form.ShapeManager.Shapes.OfType<WinOneStepShape>().Count();
        public WinOneStepValue StepAt(Point point) => StepValue(form.ShapeManager.ProbeStepAt(ToWorld(point)));
        public WinOneStepValue Step(Guid id) => StepValue(form.ShapeManager.Shapes.OfType<WinOneStepShape>().SingleOrDefault(shape => shape.Id == id));
        // Adds the next numbered badge; the number follows the existing steps.
        public WinOneStepValue AddStep(Point center, float fontPixels, Color? color = null) =>
            StepValue(form.ShapeManager.ProbeAddStep(ToWorld((PointF)center), fontPixels, color ?? Color.FromArgb(0xe2, 0x3b, 0x30)));
        // While editing, the bubble follows the inline editor's size. Null ends
        // the edit and shows the committed label again.
        public void EditStep(Guid id, Size? textSize) => form.ShapeManager.ProbeEditStep(id, textSize is { } size ? (SizeF)size : (SizeF?)null);
        // amend folds the first label of a new step into the undo step that created it.
        public void UpdateStep(Guid id, string text, Size textSize, WinOneTextLine[] lines, bool amend = false) =>
            form.ShapeManager.ProbeUpdateStep(id, text, textSize, lines, amend);
        public bool DeleteSelected() => form.ShapeManager.ProbeDeleteSelected();
        private WinOneStepValue StepValue(WinOneStepShape shape) => shape == null ? null
            : new WinOneStepValue(shape.Id, shape.Number, shape.Text, FromWorld(shape.LabelArea), shape.FontPixels, shape.FillColor.ToArgb());
    }

    internal partial class ShapeManager
    {
        // GDI draws these lines; they need a bitmap-backed HDC, not a GDI+ image surface.
        internal static bool WinOneNativeText(BaseShape shape) =>
            shape is WinOneInlineTextShape { NativeLines: not null } || shape is WinOneStepShape { NativeLines: not null };
        internal WinOneStepShape ProbeFindStep(Guid id) => Shapes.OfType<WinOneStepShape>().SingleOrDefault(shape => shape.Id == id)
            ?? throw new InvalidOperationException("Step annotation changed");
        internal WinOneStepShape ProbeStepAt(PointF point) => Shapes.OfType<WinOneStepShape>().Reverse().FirstOrDefault(shape => shape.Intersects(point));
        internal WinOneStepShape ProbeAddStep(PointF center, float fontPixels, Color color)
        {
            if (!float.IsFinite(fontPixels) || fontPixels < 6 || fontPixels > 256 || color.A != 255) throw new ArgumentOutOfRangeException(nameof(fontPixels));
            // Region tools record no history. Add under the step tool, then
            // return to Move, as inline text does.
            CurrentTool = ShapeType.DrawingStep;
            var shape = new WinOneStepShape { Manager = this };
            shape.OnConfigLoad();
            shape.FontPixels = fontPixels; shape.FillColor = color;
            shape.Place(center);
            AddShape(shape); shape.OnCreated();
            // Placing a number does not select it; only Move picks a step for Delete.
            DeselectCurrentShape();
            CurrentTool = ShapeType.ToolSelect;
            Form.Invalidate();
            return shape;
        }
        internal void ProbeEditStep(Guid id, SizeF? textSize)
        {
            if (textSize is { } size && (size.Width < 1 || size.Height < 1)) throw new ArgumentException("Invalid step label size");
            ProbeFindStep(id).EditingSize = textSize;
            InvalidateWinOneComposition(); Form.Invalidate();
        }
        internal void ProbeUpdateStep(Guid id, string text, SizeF textSize, WinOneTextLine[] lines, bool amend)
        {
            var shape = ProbeFindStep(id);
            if (text == null || text.Length > 4096 || textSize.Width < 1 || textSize.Height < 1) throw new ArgumentException("Invalid step label");
            // A blank label leaves the numbered badge on its own.
            var labelled = !string.IsNullOrWhiteSpace(text);
            if (labelled && lines == null) throw new ArgumentException("Missing step label layout");
            if (!labelled) { text = ""; lines = null; }
            shape.EditingSize = null;
            var bounds = shape.BoundsFor(labelled ? textSize : (SizeF?)null);
            var sameLines = shape.NativeLines == null ? lines == null : lines != null && shape.NativeLines.SequenceEqual(lines);
            if (shape.Text == text && shape.Rectangle == bounds && sameLines) { InvalidateWinOneComposition(); Form.Invalidate(); return; }
            if (!amend) { CurrentTool = ShapeType.DrawingStep; history.CreateShapesMemento(); }
            shape.Text = text; shape.NativeLines = lines; shape.Rectangle = bounds;
            CurrentTool = ShapeType.ToolSelect;
            OnImageModified(); Form.Invalidate();
        }
        // Delete removes the selected annotation: the one picked with Move, or the one just drawn.
        internal bool ProbeDeleteSelected()
        {
            var shape = CurrentShape;
            if (shape == null || !shape.IsHandledBySelectTool || IsCreating || IsMoving || IsResizing) return false;
            DeleteShape(shape); Form.Invalidate();
            return true;
        }
    }
}
