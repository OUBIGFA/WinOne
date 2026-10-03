namespace WinOne.Capture;

internal enum NativeSelectionAction { Copy, Save, Pin, SaveAndCopy }

// The caller owns the immutable frame; toolbar controls are never included in
// the returned pixels. Editor requests use the same selected desktop rectangle.
internal class NativeSelectionWindow : Form
{
    private readonly CapturedFrame frame;
    private readonly string prompt;
    private readonly Panel toolbar = new() { Visible = false, Cursor = Cursors.Default, TabStop = false };
    private readonly List<Button> actions = [];
    private readonly bool allowEdit;
    private readonly bool quickSave;
    private readonly bool directEdit;
    private readonly bool directConfirm;
    [System.ComponentModel.DesignerSerializationVisibility(System.ComponentModel.DesignerSerializationVisibility.Hidden)]
    internal Action<Rectangle>? BeforeEditorHandoff { get; set; }
    private readonly Func<NativeSelectionAction, Rectangle, bool>? executeAction;
    private bool executingAction;
    private string? feedback;
    private IReadOnlyList<Rectangle>? windows;
    internal event Action<Point>? WindowHovered;
    internal void UpdateWindowRegions(IReadOnlyList<Rectangle> regions)
    {
        if (IsDisposed) return;
        windows = regions;
        // Once the mouse is down, keep the user's chosen bounds stable.
        if (!dragging && !windowLocked)
        {
            Selection = SelectionGeometry.WindowAtPoint(regions, Cursor.Position, frame.Bounds);
            Invalidate();
        }
    }
    private bool windowLocked;
    private Font? toolbarFont;
    private Point anchor;
    private bool dragging;
    private bool regionDrag;
    private SelectionEdge resizeEdge;
    private Rectangle originalSelection;
    private bool shown;
    public Rectangle Selection { get; private set; }
    public string? CancellationReason { get; private set; }
    public bool EditRequested { get; private set; }

    public NativeSelectionWindow(CapturedFrame frame, string language = "zh-CN", bool dark = false, bool allowEdit = true, IReadOnlyList<Rectangle>? windows = null, Func<NativeSelectionAction, Rectangle, bool>? executeAction = null, bool quickSave = false, bool directEdit = false, bool directConfirm = false)
    {
        this.frame = frame;
        this.quickSave = quickSave;
        if (directEdit && (quickSave || !allowEdit)) throw new ArgumentException("Direct editing requires editor mode");
        this.directEdit = directEdit;
        this.directConfirm = directConfirm;
        if (quickSave && executeAction is null) throw new ArgumentException("Quick save requires a save handler", nameof(executeAction));
        if (quickSave) allowEdit = false;
        this.allowEdit = allowEdit;
        this.executeAction = executeAction;
        this.windows = windows;
        var english = language.StartsWith("en", StringComparison.OrdinalIgnoreCase);
        prompt = english
            ? (windows is null ? "Drag to select" : "Edge: window; inside: region; drag: area") + " · Enter to confirm" + (allowEdit ? " · E to edit" : "") + " · Esc to cancel"
            : (windows is null ? "拖动选择" : "边缘选整窗 · 内部选区域 · 拖动框选") + " · Enter 确认" + (allowEdit ? " · E 编辑" : "") + " · Esc 取消";
        if (quickSave) prompt = (english ? (windows is null ? "Drag an area" : "Edge: window; inside: region; drag: area") : (windows is null ? "拖动选区" : "边缘选整窗 · 内部选区域 · 拖动框选")) + (english ? " · Double-click or Enter to save · Esc to cancel" : " · 双击或 Enter 保存 · Esc 取消");
        else if (executeAction is not null) prompt = (english ? (windows is null ? "Drag an area" : "Edge: window; inside: region; drag: area") : (windows is null ? "拖动选区" : "边缘选整窗 · 内部选区域 · 拖动框选")) + (english ? " · Double-click to save and copy · E to edit · Esc to cancel" : " · 双击保存并复制 · E 编辑 · Esc 取消");
        if (directEdit) prompt = (english ? (windows is null ? "Drag an area" : "Edge: window; inside: region; drag: area") + " · Esc to cancel" : (windows is null ? "拖动选区" : "边缘选整窗 · 内部选区域 · 拖动框选") + " · Esc 取消");
        if (directConfirm) prompt = english ? "Drag the scrolling area · Esc to cancel" : "拖动选择滚动区域 · Esc 取消";
        AutoScaleMode = AutoScaleMode.None;
        FormBorderStyle = FormBorderStyle.None;
        StartPosition = FormStartPosition.Manual;
        ShowInTaskbar = false;
        TopMost = true;
        DoubleBuffered = true;
        KeyPreview = true;
        Cursor = Cursors.Cross;
        Bounds = frame.Bounds;
        Text = "WinOne";
        AccessibleDescription = prompt;
        Font = SystemFonts.MessageBoxFont;
        // Editor mode hands the selected frame to WinOne immediately. No native
        // action controls are constructed or exposed in this path.
        if (directEdit || directConfirm) return;
        var panel = dark ? Color.FromArgb(44, 44, 42) : Color.White;
        var text = dark ? Color.FromArgb(238, 238, 238) : Color.FromArgb(36, 36, 36);
        var line = dark ? Color.FromArgb(69, 69, 65) : Color.FromArgb(215, 215, 212);
        if (SystemInformation.HighContrast) { panel = SystemColors.Window; text = SystemColors.WindowText; line = SystemColors.WindowFrame; }
        toolbar.BackColor = panel;
        void Add(string name, string label, Action action, bool primary = false)
        {
            var button = new Button { Name = name, Text = label, AccessibleName = label, FlatStyle = FlatStyle.Flat, TabIndex = actions.Count, AutoEllipsis = true, UseVisualStyleBackColor = false,
                BackColor = primary ? text : panel, ForeColor = primary ? panel : text, Cursor = Cursors.Default };
            button.FlatAppearance.BorderColor = line;
            button.FlatAppearance.MouseOverBackColor = SystemInformation.HighContrast ? SystemColors.Highlight : primary ? (dark ? Color.FromArgb(215, 215, 212) : Color.FromArgb(60, 60, 58)) : (dark ? Color.FromArgb(60, 60, 58) : Color.FromArgb(231, 231, 227));
            button.Click += (_, _) => action();
            actions.Add(button); toolbar.Controls.Add(button);
        }
        Add("selection-confirm", english ? "Confirm" : "确认", () => Finish(false), true);
        if (executeAction is not null)
        {
            Add("selection-copy", english ? "Copy" : "复制", () => FinishAction(NativeSelectionAction.Copy));
            Add("selection-save", english ? "Save" : "保存", () => FinishAction(NativeSelectionAction.Save));
            Add("selection-pin", english ? "Pin screenshot" : "截图置顶", () => FinishAction(NativeSelectionAction.Pin));
        }
        if (allowEdit) Add("selection-edit", english ? "Edit" : "编辑", () => Finish(true));
        Add("selection-cancel", english ? "Cancel" : "取消", () => CancelSelection("cancelled"));
        Controls.Add(toolbar);
    }

    protected override void OnShown(EventArgs e)
    {
        base.OnShown(e);
        shown = true;
        if (ClientSize != frame.Bounds.Size || Location != frame.Bounds.Location) { CancelSelection("bounds_changed"); return; }
        Activate();
        toolbarFont = new Font("Segoe UI", 14 * DeviceDpi / 96f, FontStyle.Regular, GraphicsUnit.Pixel);
        foreach (var button in actions) button.Font = toolbarFont;
        if (windows is not null) Selection = SelectionGeometry.WindowAtPoint(windows, Cursor.Position, frame.Bounds);
        WindowHovered?.Invoke(Cursor.Position);
    }

    private Point DesktopPoint(Point point) => new(
        (int)Math.Clamp((long)frame.Bounds.X + point.X, int.MinValue, int.MaxValue),
        (int)Math.Clamp((long)frame.Bounds.Y + point.Y, int.MinValue, int.MaxValue));

    protected override void OnMouseDown(MouseEventArgs e)
    {
        base.OnMouseDown(e);
        if (executingAction) return;
        feedback = null;
        if (e.Button == MouseButtons.Right) { CancelSelection("cancelled"); return; }
        if (e.Button != MouseButtons.Left) return;
        if (e.Clicks > 1 && windowLocked) return;
        toolbar.Visible = false;
        ActiveControl = null;
        anchor = DesktopPoint(e.Location);
        originalSelection = Selection;
        resizeEdge = windowLocked ? EdgeAt(anchor) : SelectionEdge.None;
        var keepSelection = windowLocked && Selection.Contains(anchor);
        regionDrag = windows is null && !keepSelection;
        if (resizeEdge == SelectionEdge.None)
            if (!keepSelection) Selection = windows is null ? Rectangle.Empty : SelectionGeometry.WindowAtPoint(windows, anchor, frame.Bounds);
        dragging = true;
        Capture = true;
        Invalidate();
    }
    protected override void OnMouseMove(MouseEventArgs e)
    {
        base.OnMouseMove(e);
        var point = DesktopPoint(e.Location);
        if (!dragging)
        {
            if (windows is not null && !windowLocked) { WindowHovered?.Invoke(point); Selection = SelectionGeometry.WindowAtPoint(windows, point, frame.Bounds); Invalidate(); }
            Cursor = EdgeCursor(windowLocked ? EdgeAt(point) : SelectionEdge.None);
            return;
        }
        UpdateDrag(point);
        Invalidate();
    }
    protected override void OnMouseUp(MouseEventArgs e)
    {
        base.OnMouseUp(e);
        if (!dragging || e.Button != MouseButtons.Left) return;
        UpdateDrag(DesktopPoint(e.Location));
        dragging = false;
        windowLocked = !Selection.IsEmpty;
        Capture = false;
        if ((directEdit || directConfirm) && windowLocked) { Finish(directEdit); return; }
        ShowToolbar();
        Invalidate();
    }
    private SelectionEdge EdgeAt(Point point) => SelectionGeometry.HitEdge(Selection, point, Math.Max(4, (int)(6 * DeviceDpi / 96f)), frame.Bounds);
    protected override void OnMouseDoubleClick(MouseEventArgs e)
    {
        base.OnMouseDoubleClick(e);
        if (e.Button == MouseButtons.Left && windowLocked && !dragging)
            FinishAction(quickSave ? NativeSelectionAction.Save : NativeSelectionAction.SaveAndCopy);
    }
    private static Cursor EdgeCursor(SelectionEdge edge) => edge switch
    {
        SelectionEdge.Left or SelectionEdge.Right => Cursors.SizeWE,
        SelectionEdge.Top or SelectionEdge.Bottom => Cursors.SizeNS,
        SelectionEdge.TopLeft or SelectionEdge.BottomRight => Cursors.SizeNWSE,
        SelectionEdge.TopRight or SelectionEdge.BottomLeft => Cursors.SizeNESW,
        _ => Cursors.Cross
    };
    private void UpdateDrag(Point point)
    {
        if (resizeEdge != SelectionEdge.None) { Selection = SelectionGeometry.Resize(originalSelection, resizeEdge, point, frame.Bounds); return; }
        // Use the system drag threshold so ordinary click jitter still selects a window.
        if (Math.Abs((long)point.X - anchor.X) * 2 >= SystemInformation.DragSize.Width || Math.Abs((long)point.Y - anchor.Y) * 2 >= SystemInformation.DragSize.Height) regionDrag = true;
        if (regionDrag) Selection = SelectionGeometry.FromEdges(anchor, point, frame.Bounds);
    }
    protected override void OnKeyDown(KeyEventArgs e)
    {
        base.OnKeyDown(e);
        if (executingAction) return;
        if (executeAction is not null && e.Control && e.KeyCode is Keys.C or Keys.S)
        { e.Handled = true; e.SuppressKeyPress = true; FinishAction(e.KeyCode == Keys.C ? NativeSelectionAction.Copy : NativeSelectionAction.Save); return; }
        if (e.KeyCode == Keys.Escape) { e.Handled = true; CancelSelection("cancelled"); }
        else if (e.KeyCode == Keys.Enter && ActiveControl is not Button && !dragging)
        { e.Handled = true; e.SuppressKeyPress = true; Finish(false); }
        else if (e.KeyCode == Keys.E && allowEdit && !dragging)
        { e.Handled = true; e.SuppressKeyPress = true; Finish(true); }
    }
    protected override void OnDeactivate(EventArgs e)
    {
        base.OnDeactivate(e);
        if (shown && !executingAction && DialogResult == DialogResult.None) CancelSelection("deactivated");
    }
    protected override void OnDpiChanged(DpiChangedEventArgs e)
    {
        if (shown) { CancelSelection("dpi_changed"); return; }
        base.OnDpiChanged(e);
    }
    protected override void WndProc(ref Message message)
    {
        if (shown && message.Msg == 0x007E) { CancelSelection("display_changed"); return; }
        base.WndProc(ref message);
    }
    private void CancelSelection(string reason)
    {
        CancellationReason = reason;
        Selection = Rectangle.Empty;
        dragging = false;
        EditRequested = false;
        Capture = false;
        DialogResult = DialogResult.Cancel;
        Close();
    }
    private void Finish(bool edit)
    {
        if (dragging || Selection.Width <= 0 || Selection.Height <= 0) return;
        if (quickSave) { FinishAction(NativeSelectionAction.Save); return; }
        if (directEdit)
        {
            // Showing the retained frame may deactivate this modal selector.
            // Treat the handoff like an owned action, not a user cancellation.
            executingAction = true; feedback = null;
            try { BeforeEditorHandoff?.Invoke(Selection); }
            catch (Exception error) { Console.Error.WriteLine(error); feedback = error.Message; AccessibleDescription = feedback; Invalidate(); return; }
            finally {
                executingAction = false;
                // A failed handoff keeps the selection available for retry/Esc.
                if (feedback is not null && Visible && DialogResult == DialogResult.None) Activate();
            }
            // Display/DPI changes still cancel; never overwrite that cancellation.
            if (CancellationReason is not null) return;
        }
        EditRequested = edit || directEdit;
        DialogResult = DialogResult.OK;
        Close();
    }
    private void FinishAction(NativeSelectionAction action)
    {
        if (executeAction is null || executingAction || dragging || Selection.IsEmpty) return;
        executingAction = true; feedback = null;
        try
        {
            // Keep the immutable frame and region alive on failure or save
            // cancellation. An owned dialog may temporarily deactivate us.
            if (executeAction(action, Selection)) { DialogResult = DialogResult.OK; Close(); }
        }
        catch (Exception error) { Console.Error.WriteLine(error); feedback = error.Message; AccessibleDescription = feedback; }
        finally
        {
            executingAction = false;
            if (!IsDisposed && DialogResult == DialogResult.None) { Activate(); Invalidate(); }
        }
    }
    private void ShowToolbar()
    {
        if (quickSave) return;
        if (Selection.IsEmpty) return;
        var monitor = Rectangle.Intersect(frame.Bounds, Screen.FromPoint(Cursor.Position).WorkingArea);
        if (monitor.IsEmpty) monitor = frame.Bounds;
        var scale = DeviceDpi / 96f;
        var bounds = SelectionGeometry.ToolbarBounds(Selection, monitor, new Size((int)(actions.Count * 92 * scale + 16 * scale), (int)(48 * scale)), (int)(8 * scale));
        bounds.Offset(-frame.Bounds.X, -frame.Bounds.Y);
        toolbar.Bounds = bounds;
        var padding = Math.Min((int)(6 * scale), Math.Max(0, bounds.Height / 4));
        var gap = Math.Min((int)(6 * scale), Math.Max(0, bounds.Width / 16));
        var width = Math.Max(1, (bounds.Width - padding * 2 - gap * (actions.Count - 1)) / actions.Count);
        for (var i = 0; i < actions.Count; i++) actions[i].Bounds = new Rectangle(padding + i * (width + gap), padding, width, Math.Max(1, bounds.Height - padding * 2));
        toolbar.Visible = true;
        toolbar.BringToFront();
    }
    protected override void Dispose(bool disposing)
    {
        base.Dispose(disposing);
        if (disposing) toolbarFont?.Dispose();
    }
    protected override void OnPaint(PaintEventArgs e)
    {
        e.Graphics.PageUnit = GraphicsUnit.Pixel;
        frame.DrawUnscaled(e.Graphics);
        var local = Selection;
        local.Offset(-frame.Bounds.X, -frame.Bounds.Y);
        using var shade = new Region(ClientRectangle);
        if (Selection.Width > 0 && Selection.Height > 0) shade.Exclude(local);
        using var dim = new SolidBrush(Color.FromArgb(100, 0, 0, 0));
        e.Graphics.FillRegion(dim, shade);
        if (Selection.Width > 0 && Selection.Height > 0)
        {
            using var light = new Pen(Color.FromArgb(231, 231, 226), 3);
            using var dark = new Pen(Color.FromArgb(44, 44, 42), 1);
            e.Graphics.DrawRectangle(light, local.X, local.Y, local.Width - 1, local.Height - 1);
            e.Graphics.DrawRectangle(dark, local.X, local.Y, local.Width - 1, local.Height - 1);
            if (windowLocked && Selection != frame.Bounds && !dragging)
            {
                var side = Math.Max(4, (int)(6 * DeviceDpi / 96f));
                // Handles are selection affordances only; export uses the immutable frame.
                foreach (var point in new[] { new Point(local.Left, local.Top), new Point(local.Right, local.Top), new Point(local.Left, local.Bottom), new Point(local.Right, local.Bottom),
                    new Point(local.Left + local.Width / 2, local.Top), new Point(local.Left + local.Width / 2, local.Bottom), new Point(local.Left, local.Top + local.Height / 2), new Point(local.Right, local.Top + local.Height / 2) })
                {
                    var handle = new Rectangle(point.X - side / 2, point.Y - side / 2, side, side);
                    e.Graphics.FillRectangle(Brushes.White, handle); e.Graphics.DrawRectangle(dark, handle);
                }
            }
        }
        var monitor = Rectangle.Intersect(frame.Bounds, Screen.FromPoint(Cursor.Position).Bounds);
        if (monitor.IsEmpty) monitor = frame.Bounds;
        var text = feedback ?? (Selection.Width > 0 && Selection.Height > 0 ? $"{Selection.Width} × {Selection.Height}  ·  {prompt}" : prompt);
        var width = Math.Min(TextRenderer.MeasureText(text, Font).Width + 24, Math.Max(1, monitor.Width - 32));
        var banner = new Rectangle(monitor.X - frame.Bounds.X + 16, monitor.Y - frame.Bounds.Y + 16, width, 36);
        using var background = new SolidBrush(Color.FromArgb(44, 44, 42));
        e.Graphics.FillRectangle(background, banner);
        TextRenderer.DrawText(e.Graphics, text, Font, banner, Color.White, TextFormatFlags.HorizontalCenter | TextFormatFlags.VerticalCenter | TextFormatFlags.EndEllipsis);
    }
}
