using System;
using System.Drawing;
using System.Drawing.Text;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;

namespace ShareX.ScreenCaptureLib
{
    // WinOne facade compiled alongside the locked native drawing engine.
    // Existing selection, drawing, effects and history remain engine-owned.
    public sealed partial class WinOneCaptureCanvas : IDisposable
    {
        private readonly RegionCaptureForm form;
        private string selectedTool = "region";
        private Rectangle[] windowCandidates = Array.Empty<Rectangle>();
        public Rectangle[] ActiveWindowCandidates => form.ShapeManager.Windows?.Select(window => window.Rectangle).ToArray() ?? Array.Empty<Rectangle>();
        public string SelectedTool => selectedTool;
        public bool RegionGestureActive => selectedTool == "region" && (form.ShapeManager.IsCreating || form.ShapeManager.IsMoving || form.ShapeManager.IsResizing);
        public Form Window => form;
        public int AnnotationCount => form.ShapeManager.Shapes.Count(shape => shape.ShapeCategory != ShapeCategory.Region);
        public bool CanUndo => form.ShapeManager.ProbeCanUndo;
        public bool CanRedo => form.ShapeManager.ProbeCanRedo;
        public Rectangle? Selection => form.ShapeManager.ProbeCommittedRegion is BaseShape region ? FromWorld(region.Rectangle) : (Rectangle?)null;
        public int SelectionCount => form.ShapeManager.ValidRegions.Length;
        public string Tool => form.ShapeManager.CurrentTool.ToString();
        public WinOneTextValue EditableText => form.ShapeManager.CurrentShape is WinOneInlineTextShape text
            ? new WinOneTextValue(text.Id, text.Text, FromWorld(text.Rectangle), text.FontPixels, text.TextColor.ToArgb()) : null;
        public WinOneTextValue TextAt(Point point) => form.ShapeManager.ProbeTextAt(ToWorld(point)) is WinOneInlineTextShape text
            ? new WinOneTextValue(text.Id, text.Text, FromWorld(text.Rectangle), text.FontPixels, text.TextColor.ToArgb()) : null;
        public event Action Changed;
        public event Action DocumentChanged;
        public event Action<Exception> RenderFailed;
        public event Action<Keys> KeyPressed;
        public event Action<Point> PointerMoved;
        public event Action<string> ToolChanged;
        public event Action RegionCommitted;

        public WinOneCaptureCanvas(Bitmap image, Action<string> startupStage = null, bool viewport = false)
        {
            var canvas = new Bitmap(image);
            startupStage?.Invoke("canvasCopied");
            form = new RegionCaptureForm(viewport ? RegionCaptureMode.Editor : RegionCaptureMode.Annotation, new RegionCaptureOptions {
                DetectWindows = false, DetectControls = false, EnableSharing = false,
                QuickCrop = false, InputDelay = 0, EnableAnimations = false,
                EditorAutoCopyImage = false,
                ImageEditorStartMode = ImageEditorStartMode.Fullscreen, ZoomToFitOnOpen = true, ShowEditorPanTip = false,
            }, canvas);
            form.EnableWinOnePresentation();
            form.ShapeManager.AnnotationOptions.Shadow = false;
            startupStage?.Invoke("nativeFormCreated");
            form.ShapeManager.UseHostedToolbarForProbe();
            form.ShapeManager.UseHostedKeyboardForProbe(key => KeyPressed?.Invoke(key));
            form.ShapeManager.ShapeCreated += form.ShapeManager.ProbeCommitSingleRegion;
            form.ShapeManager.ShapeCreated += shape => {
                if (shape.ShapeCategory == ShapeCategory.Region && shape.IsValidShape) {
                    form.Options.ShowMagnifier = false;
                    RegionCommitted?.Invoke();
                }
            };
            form.ShapeManager.ImageModified += () => {
                form.ShapeManager.InvalidateWinOneComposition();
                // Precision magnification belongs only to choosing the initial
                // region; committed captures stay unobstructed while editing.
                if (Selection is not null) form.Options.ShowMagnifier = false;
                Changed?.Invoke(); DocumentChanged?.Invoke();
            };
            form.ShapeManager.WinOneCompositionFailed += error => RenderFailed?.Invoke(error);
            form.Disposed += (_,_)=>form.ShapeManager.DisposeWinOneComposition();
            form.ShapeManager.CurrentShapeTypeChanged += _ => Changed?.Invoke();
            form.MouseMove += (_, mouse) => PointerMoved?.Invoke(form.PointToScreen(mouse.Location));
            if (viewport) {
                var previousScale = form.ZoomFactor;
                var previousOrigin = form.CanvasRectangle.Location;
                void ViewChanged() {
                    if (previousScale == form.ZoomFactor && previousOrigin == form.CanvasRectangle.Location) return;
                    previousScale = form.ZoomFactor; previousOrigin = form.CanvasRectangle.Location;
                    Changed?.Invoke();
                }
                form.MouseMove += (_, _) => ViewChanged();
                form.MouseWheel += (_, _) => ViewChanged();
                form.Resize += (_, _) => ViewChanged();
                form.Shown += (_, _) => ViewChanged();
            }
        }

        public void SelectTool(string tool)
        {
            var nativeTool = tool switch {
                "region" => ShapeType.RegionRectangle,
                "select" => ShapeType.ToolSelect,
                "rectangle" => ShapeType.DrawingRectangle,
                "cover" => ShapeType.DrawingRectangle,
                "ellipse" => ShapeType.DrawingEllipse,
                "pen" => ShapeType.DrawingFreehand,
                "arrow" => ShapeType.DrawingArrow,
                "mosaic" => ShapeType.EffectPixelate,
                "blur" => ShapeType.EffectBlur,
                _ => throw new ArgumentException("Unsupported prototype tool", nameof(tool))
            };
            FinishGesture();
            selectedTool = tool;
            form.ShapeManager.CurrentTool = nativeTool;
            // New annotations use flat strokes. A restored layer may carry
            // legacy appearance data, but must not change the next tool's style.
            form.ShapeManager.AnnotationOptions.Shadow = false;
            ApplyWindowCandidates();
            ToolChanged?.Invoke(tool);
            form.Invalidate();
        }
        public void UpdateWindowCandidates(Rectangle[] rectangles)
        {
            if (rectangles.Any(rectangle => rectangle.Width <= 0 || rectangle.Height <= 0)) throw new ArgumentException("Invalid window region");
            windowCandidates = rectangles.Distinct().ToArray(); ApplyWindowCandidates(); form.RefreshWinOneInput(); form.Invalidate(); Changed?.Invoke();
        }
        private void ApplyWindowCandidates()
        {
            // Window snapping belongs to region selection. Leaving it enabled
            // for rectangle/cover tools can turn a click into a full-window mark.
            form.ShapeManager.Windows = selectedTool == "region"
                ? windowCandidates.Select(rectangle => new SimpleWindowInfo(IntPtr.Zero, rectangle)).ToList() : null;
        }
        public void Undo() { var selection = Selection; form.ShapeManager.ProbeUndo(); RestoreSelection(selection); form.Invalidate(); }
        public void Redo() { var selection = Selection; form.ShapeManager.ProbeRedo(); RestoreSelection(selection); form.Invalidate(); }
        private void RestoreSelection(Rectangle? selection) => form.ShapeManager.ProbeRestoreSelection(selection is { } value ? ToWorld(value) : (RectangleF?)null);
        public Bitmap Render() => form.ShapeManager.ProbeRenderOutput(form.Canvas);
        public Task PrepareRenderAsync(CancellationToken cancellation = default) => form.ShapeManager.PrepareWinOneCompositionAsync(cancellation);
        public Bitmap RenderSelection()
        {
            var region = Selection ?? throw new InvalidOperationException("请先选择截图区域");
            using var rendered = Render();
            region = Rectangle.Intersect(region, new Rectangle(Point.Empty, rendered.Size));
            if (region.Width < 1 || region.Height < 1) throw new InvalidOperationException("Invalid selection");
            return rendered.Clone(region, rendered.PixelFormat);
        }
        public void SetRegion(Rectangle bounds) => form.ShapeManager.ProbeAddRegion(ToWorld(bounds));
        public void BeginRegion(Rectangle bounds) => form.ShapeManager.ProbeBeginRegion(ToWorld(bounds));
        public void AddRectangle(Rectangle bounds) { SelectTool("rectangle"); form.ShapeManager.ProbeAddRectangle(ToWorld(bounds)); }
        public void AddDrawing(string tool, RectangleF bounds, PointF[] points = null) { SelectTool(tool); form.ShapeManager.ProbeAddDrawing(ToWorld(bounds), points?.Select(ToWorld).ToArray(), tool == "cover"); }
        public void BeginCover(Point point) { if (selectedTool == "cover") form.ShapeManager.ProbeBeginCover(ToWorld(point)); }
        public void AddText(string text, Rectangle bounds, float fontSize, WinOneTextLine[] lines = null, Color? color = null) => form.ShapeManager.ProbeAddText(text, ToWorld(bounds), fontSize, lines, color);
        public void SetTextEditing(bool editing) { form.ShapeManager.NodesVisible = !editing; form.Invalidate(); }
        public WinOneToolStyle GetToolStyle() => form.ShapeManager.GetWinOneStyle(selectedTool);
        public void SetToolStyle(WinOneToolStyle style) { FinishGesture(); form.ShapeManager.SetWinOneStyle(selectedTool, style); Changed?.Invoke(); DocumentChanged?.Invoke(); }
        public WinOneCaptureDocument ExportDocument() { FinishGesture(); return form.ShapeManager.ExportWinOneDocument(selectedTool); }
        // Background-save snapshots never finish an in-progress pointer gesture.
        // The UI retries after its normal mouse/key dispatch completes.
        public WinOneCaptureDocument SnapshotDocument() => form.ShapeManager.IsCreating || form.ShapeManager.IsMoving || form.ShapeManager.IsResizing
            ? null : form.ShapeManager.ExportWinOneDocument(selectedTool);
        public void RestoreDocument(WinOneCaptureDocument document) { form.ShapeManager.RestoreWinOneDocument(document); if (Selection is not null) form.Options.ShowMagnifier = false; SelectTool(document.Tool); Changed?.Invoke(); }
        public void FinishGesture() { if (form.ShapeManager.IsCreating || form.ShapeManager.IsMoving) form.ShapeManager.EndRegionSelection(); }
        public void HideEditedText(Guid id, bool hidden) { form.ShapeManager.ProbeFindText(id).Editing = hidden; form.ShapeManager.InvalidateWinOneComposition(); form.Invalidate(); }
        public void UpdateText(Guid id, string text, Rectangle bounds, WinOneTextLine[] lines = null, float? fontPixels = null, Color? color = null) => form.ShapeManager.ProbeUpdateText(id, text, ToWorld(bounds), lines, fontPixels, color);
        public DialogResult ShowDialog() => form.ShowDialog();
        public void Dispose() => form.Dispose();
    }


    internal partial class ShapeManager
    {
        internal WinOneToolStyle GetWinOneStyle(string tool)
        {
            var value = tool == "mosaic" ? AnnotationOptions.PixelateSize : tool == "blur" ? AnnotationOptions.BlurRadius : AnnotationOptions.BorderSize;
            var color = AnnotationOptions.BorderColor;
            if (CurrentShape?.ShapeType == CurrentTool) {
                if (CurrentShape is PixelateEffectShape pixel) value = pixel.PixelSize;
                else if (CurrentShape is BlurEffectShape blur) value = blur.BlurRadius;
                else if (CurrentShape is BaseDrawingShape drawing) { value = drawing.BorderSize; color = drawing.BorderColor; }
            }
            return new WinOneToolStyle(color, value){Precision=CurrentShape?.ShapeType==CurrentTool&&CurrentShape is IWinOneLegacyStroke legacy?legacy.LegacyStrokeWidth:null};
        }
        internal void SetWinOneStyle(string tool, WinOneToolStyle style)
        {
            if (tool is not ("rectangle" or "ellipse" or "arrow" or "pen" or "cover" or "mosaic" or "blur")) throw new ArgumentException("Tool has no drawing style");
            var minimum = tool == "mosaic" ? 2 : tool == "blur" ? 2 : tool == "cover" ? 0 : 1;
            var maximum = tool == "mosaic" ? 10000 : tool == "blur" ? 199 : 64;
            if (!float.IsFinite(style.EffectiveValue) || style.EffectiveValue < minimum || style.EffectiveValue > maximum || style.Color.A != 255) throw new ArgumentOutOfRangeException(nameof(style));
            if (GetWinOneStyle(tool) == style) return;
            // Defaults affect the next mark; only the selected matching mark
            // changes now. Its prior pixels remain recoverable through history.
            var shape = CurrentShape?.ShapeType == CurrentTool ? CurrentShape : null;
            if (shape is not null) history.CreateShapesMemento();
            if (tool == "mosaic") AnnotationOptions.PixelateSize = style.Value;
            else if (tool == "blur") AnnotationOptions.BlurRadius = style.Value;
            else { AnnotationOptions.BorderColor = style.Color; AnnotationOptions.BorderSize = style.Value; }
            if (shape is PixelateEffectShape pixel) pixel.PixelSize = style.Value;
            else if (shape is BlurEffectShape blur) blur.BlurRadius = style.Value;
            else if (shape is BaseDrawingShape drawing) {
                drawing.BorderColor = style.Color; drawing.BorderSize = style.Value;
                if(shape is IWinOneLegacyStroke legacy)legacy.LegacyStrokeWidth=style.EffectiveValue;
                if (shape is WinOneCoverShape cover) cover.FillColor = style.Color;
            }
            // Changing width/color must not reset restored arrow control points,
            // fills or shadows from unrelated global defaults.
            shape?.OnMoved();
            OnImageModified(); Form.Invalidate();
        }
        // The old region remains committed while a replacement is being drawn.
        // Do not expose an in-progress second region as a completed selection.
        internal BaseShape ProbeCommittedRegion => ValidRegions.Where(region => !IsCreating || region != CurrentShape).SingleOrDefault();
        internal void ProbeBeginRegion(RectangleF bounds)
        {
            CurrentTool = ShapeType.RegionRectangle;
            var shape = CreateShape(CurrentTool); shape.Rectangle = bounds;
            AddShape(shape); IsCreating = true;
        }
        internal Bitmap ProbeRenderOutput(Bitmap source)
        {
            if(WinOneOrderedEffects)return RenderWinOneOrdered(source);
            var offset = Form.CanvasRectangle.Location;
            if (!DrawingShapes.OfType<WinOneInlineTextShape>().Any(shape => shape.NativeLines != null)) return RenderOutputImage(source, offset);
            using var background = (Bitmap)source.Clone();
            IsRenderingOutput = true; RenderOffset = offset;
            MoveAll(-offset.X, -offset.Y);
            try {
                using (var graphics = Graphics.FromImage(background))
                    foreach (var effect in EffectShapes) effect.OnDrawFinal(graphics, background);
                return NativeDrawingSurface.Draw(background, graphics => {
                    foreach (var drawing in DrawingShapes) { drawing.OnDraw(graphics); graphics.Flush(); }
                });
            } finally { MoveAll(offset); RenderOffset = Point.Empty; IsRenderingOutput = false; }
        }
        // The original toolbar is optional UI, not the drawing/history owner.
        internal void UseHostedToolbarForProbe() => Form.Shown -= form_Shown;
        internal void UseHostedKeyboardForProbe(Action<Keys> command)
        {
            // Replace both legacy command owners. Keep only geometry key state
            // in the engine; clipboard, history and tools belong to WinOne.
            Form.KeyDown -= Form.RegionCaptureForm_KeyDown;
            Form.KeyDown -= form_KeyDown;
            Form.KeyDown += (sender, args) => {
                if (Form.ActiveControl is TextBoxBase) return;
                switch (args.KeyCode) {
                    case Keys.ControlKey: case Keys.ShiftKey: case Keys.Menu:
                    case Keys.Left: case Keys.Right: case Keys.Up: case Keys.Down:
                        form_KeyDown(sender, args); return;
                }
                args.Handled = true; args.SuppressKeyPress = true;
                command(args.KeyData);
            };
        }
        internal bool ProbeCanUndo => history.CanUndo;
        internal bool ProbeCanRedo => history.CanRedo;
        internal void ProbeUndo() => history.Undo();
        internal void ProbeRedo() => history.Redo();
        internal void ProbeAddRegion(RectangleF bounds)
        {
            CurrentTool = ShapeType.RegionRectangle;
            var shape = CreateShape(CurrentTool);
            shape.Rectangle = bounds;
            AddShape(shape);
            shape.OnCreated();
            OnShapeCreated(shape);
        }
        // WinOne has one crop. Keep the previous crop while a gesture is
        // incomplete, and replace it only at the engine's completed-shape event.
        internal void ProbeCommitSingleRegion(BaseShape completed)
        {
            if (completed.ShapeCategory != ShapeCategory.Region || !completed.IsValidShape) return;
            foreach (var previous in Regions.Where(region => region != completed).ToArray()) {
                previous.Dispose(); Shapes.Remove(previous);
            }
        }
        internal void ProbeRestoreSelection(RectangleF? bounds)
        {
            // Upstream annotation mementos include regions, although region
            // edits do not create mementos. Restore the current crop separately.
            foreach (var region in Regions.ToArray()) { region.Dispose(); Shapes.Remove(region); }
            if (bounds is null) return;
            var selection = CreateShape(ShapeType.RegionRectangle);
            selection.Rectangle = bounds.Value;
            Shapes.Add(selection);
        }
        internal void ProbeAddRectangle(RectangleF bounds)
        {
            CurrentTool = ShapeType.DrawingRectangle;
            var shape = CreateShape(CurrentTool);
            shape.Rectangle = bounds;
            AddShape(shape);
            shape.OnCreated();
            OnImageModified();
        }
        internal void ProbeAddDrawing(RectangleF bounds, PointF[] points, bool cover)
        {
            BaseShape shape;
            if (cover) shape = ProbeCreateCover();
            else if (CurrentTool == ShapeType.DrawingFreehand) {
                var pen = new WinOneStrokeShape { Manager = this };
                pen.OnConfigLoad(); pen.SetPoints(points ?? throw new ArgumentException("Missing stroke points")); shape = pen;
            } else shape = CreateShape(CurrentTool);
            // Programmatic arrows need the same endpoints that the native
            // gesture supplies; a bounding rectangle alone draws a zero line.
            if (shape is LineDrawingShape line) {
                line.Points[0] = bounds.Location;
                line.Points[line.Points.Length - 1] = new PointF(bounds.Right, bounds.Bottom);
                line.OnUpdate();
            }
            shape.Rectangle = bounds; AddShape(shape); shape.OnCreated(); OnImageModified();
        }
        private WinOneCoverShape ProbeCreateCover()
        {
            var shape = new WinOneCoverShape { Manager = this };
            shape.OnConfigLoad(); return shape;
        }
        internal void ProbeBeginCover(PointF point)
        {
            if (IsCreating || IsCursorOnObject || GetIntersectShape(point) != null) return;
            ClearTools(); DeselectCurrentShape();
            var shape = ProbeCreateCover(); AddShape(shape);
            // Enter the same native creation state used by BaseShape.OnCreating.
            // Subsequent motion, resize, completion and history remain upstream-owned.
            IsCreating = true; shape.Rectangle = new RectangleF(point, new SizeF(1, 1));
        }
        internal void ProbeAddText(string text, RectangleF bounds, float fontSize, WinOneTextLine[] lines, Color? color)
        {
            if (string.IsNullOrWhiteSpace(text) || text.Length > 4096 || bounds.Width < 1 || bounds.Height < 1 || !float.IsFinite(fontSize) || fontSize < 6 || fontSize > 256 || color?.A is < 255)
                throw new ArgumentException("Invalid inline text");
            CurrentTool = ShapeType.DrawingTextBackground;
            var shape = new WinOneInlineTextShape { Manager = this };
            shape.OnConfigLoad();
            shape.Text = text; shape.Rectangle = bounds; shape.FontPixels = fontSize;
            shape.TextColor = color ?? Color.FromArgb(0xe2, 0x3b, 0x30);
            shape.NativeLines = lines;
            AddShape(shape); shape.OnCreated();
            CurrentTool = ShapeType.ToolSelect;
            OnImageModified();
        }
        internal WinOneInlineTextShape ProbeFindText(Guid id) => Shapes.OfType<WinOneInlineTextShape>().SingleOrDefault(shape => shape.Id == id)
            ?? throw new InvalidOperationException("Text annotation changed");
        // Restored crop regions can be last in the engine's hit-test order.
        // The text tool targets text layers, not that covering crop region.
        internal WinOneInlineTextShape ProbeTextAt(PointF point) => Shapes.OfType<WinOneInlineTextShape>().Reverse().FirstOrDefault(shape => shape.Intersects(point));
        internal void ProbeUpdateText(Guid id, string value, RectangleF bounds, WinOneTextLine[] lines, float? fontPixels, Color? color)
        {
            var shape = ProbeFindText(id);
            var nextFont = fontPixels ?? shape.FontPixels;
            var nextColor = color ?? shape.TextColor;
            if (!float.IsFinite(nextFont) || nextFont < 6 || nextFont > 256 || nextColor.A != 255) throw new ArgumentOutOfRangeException(nameof(fontPixels));
            if (value.Length > 4096 || bounds.Width < 1 || bounds.Height < 1) throw new ArgumentException("Invalid text update");
            shape.Editing = false;
            var sameLines = shape.NativeLines == null ? lines == null : lines != null && shape.NativeLines.SequenceEqual(lines);
            if (shape.Text == value && shape.Rectangle == bounds && sameLines && shape.FontPixels == nextFont && shape.TextColor.ToArgb() == nextColor.ToArgb()) return;
            CurrentTool = ShapeType.DrawingTextBackground;
            if (string.IsNullOrWhiteSpace(value)) { DeleteShape(shape); CurrentTool = ShapeType.ToolSelect; return; }
            history.CreateShapesMemento();
            shape.Text = value; shape.Rectangle = bounds;
            shape.FontPixels = nextFont; shape.TextColor = nextColor;
            shape.NativeLines = lines;
            CurrentTool = ShapeType.ToolSelect;
            OnImageModified();
        }
    }

    public sealed record WinOneToolStyle(Color Color, int Value)
    {
        public float? Precision{get;init;}
        public float EffectiveValue=>Precision??Value;
    }
    public sealed record WinOneTextValue(Guid Id, string Text, Rectangle Bounds, float FontPixels, int ColorArgb = unchecked((int)0xffe23b30));
    public sealed class WinOneCoverShape : RectangleDrawingShape
    {
        public override void OnConfigLoad()
        {
            base.OnConfigLoad(); FillColor = Color.FromArgb(255, BorderColor.R, BorderColor.G, BorderColor.B);
            BorderSize = 0; CornerRadius = 0; Shadow = false;
        }
        // A cover is an opaque fill, not a rectangle-style preset.
        public override void OnConfigSave() { }
        public override void OnDraw(Graphics graphics)
        {
            var bounds = System.Drawing.Rectangle.FromLTRB((int)Math.Floor(Rectangle.Left), (int)Math.Floor(Rectangle.Top),
                (int)Math.Ceiling(Rectangle.Right), (int)Math.Ceiling(Rectangle.Bottom));
            var saved = graphics.Save();
            try {
                graphics.SmoothingMode = System.Drawing.Drawing2D.SmoothingMode.None;
                graphics.PixelOffsetMode = System.Drawing.Drawing2D.PixelOffsetMode.None;
                using var brush = new SolidBrush(Color.FromArgb(255, FillColor.R, FillColor.G, FillColor.B));
                graphics.FillRectangle(brush, bounds);
            } finally { graphics.Restore(saved); }
        }
    }
    public sealed class WinOneStrokeShape : FreehandDrawingShape
    {
        public void SetPoints(PointF[] value) => positions = value.ToList();
    }

    public sealed class WinOneInlineTextShape : TextDrawingShape
    {
        public Guid Id { get; set; } = Guid.NewGuid();
        [Newtonsoft.Json.JsonIgnore]
        public bool Editing { get; set; }
        public float FontPixels { get; set; } = 24;
        public Color TextColor { get; set; } = Color.FromArgb(0xe2, 0x3b, 0x30);
        public WinOneTextLine[] NativeLines { get; set; }
        public override void OnCreating() => throw new InvalidOperationException("Use the inline editor");
        public override void OnCreated() => OnCreated(false);
        public override void OnDoubleClicked() { }
        public override void OnDraw(Graphics graphics)
        {
            if (Editing) return;
            if (NativeLines != null) { NativeDrawingSurface.DrawText(graphics, NativeLines, System.Drawing.Rectangle.Round(Rectangle), FontPixels, TextColor); return; }
            // Same text contract as WinOne's existing AnnotationRenderer:
            // pixel-sized Segoe UI and typographic layout, without dialog padding.
            var state = graphics.Save();
            try {
                graphics.SetClip(Rectangle);
                graphics.TextRenderingHint = TextRenderingHint.AntiAliasGridFit;
                using var font = new Font("Segoe UI", FontPixels, FontStyle.Regular, GraphicsUnit.Pixel);
                using var brush = new SolidBrush(TextColor);
                graphics.DrawString(Text, font, brush, Rectangle, StringFormat.GenericTypographic);
            } finally { graphics.Restore(state); }
        }
    }
}
