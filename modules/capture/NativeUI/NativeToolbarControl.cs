namespace WinOne.Capture.NativeUI;

// Native presentation only: commands are handled by the capture session.
// Standard ToolStrip controls own focus, accessibility and button interaction.
internal sealed class NativeToolbarControl : UserControl
{
    protected override CreateParams CreateParams {
        get {
            var value = base.CreateParams;
            // Buffer only this small child-control tree, not the full-screen
            // canvas. Parent-only double buffering cannot include ToolStrip HWNDs.
            // WS_EX_COMPOSITED presents the background and rows together.
            value.ExStyle |= 0x02000000;
            return value;
        }
    }
    private readonly GripToolStrip actions = Row();
    private readonly ToolStrip tools = Row();
    private readonly GripToolStrip grip = Row();
    private readonly ToolTip gripHint = new();
    private readonly ToolStripLabel gripIcon = new() { DisplayStyle = ToolStripItemDisplayStyle.Image, AutoSize = false, Size = new Size(24, 36), Margin = Padding.Empty, ImageAlign = ContentAlignment.MiddleCenter };
    private bool saveAsAvailable = true;
    private bool pinAvailable = true;
    private bool retakeAvailable;
    private bool scrollAvailable;
    private bool scrollEnabled;
    private bool recordingAvailable;
    private readonly bool dark;
    private readonly Color borderColor;
    private Image? overflowImage;
    private readonly Dictionary<string, ToolStripButton> buttons = new();
    private readonly HashSet<string> drawingCommands = new(StringComparer.Ordinal);
    private HashSet<string>? preferredTools;
    private readonly NativeToolbarCustomization? customization;
    private readonly Func<string[]?, Task<string[]?>>? saveToolbar;
    private bool preferencesBusy;
    public event Action? PreferencesChanged;
    public event Action? PreferencesSaving;
    public event Action<string>? PreferencesFailed;
    private readonly NativeToolOptions options;
    private readonly NativeMenuTheme menuTheme;
    public event Action<ShareX.ScreenCaptureLib.WinOneToolStyle>? ToolStyleChanged;
    public event Action<string>? Command;
    public event Action<Point>? MoveRequested;

    public NativeToolbarControl(bool dark = false, string? language = null, IReadOnlyList<string>? toolbarTools = null, Func<string[]?, Task<string[]?>>? saveToolbar = null)
    {
        this.dark = dark;
        borderColor = new NativeMenuColors(dark: dark).ToolStripBorder;
        SetStyle(ControlStyles.ResizeRedraw, true);
        this.saveToolbar = saveToolbar;
        if (toolbarTools is not null) {
            var allowed = new HashSet<string>(new[] { "select", "crop", "rectangle", "ellipse", "arrow", "pen", "cover", "text", "mosaic", "blur", "ocr", "translate" }, StringComparer.Ordinal);
            if (toolbarTools.Any(tool => !allowed.Remove(tool))) throw new ArgumentException("Invalid toolbar preferences", nameof(toolbarTools));
            // Preserve the existing move identifier; crop maps to the native region tool.
            preferredTools = toolbarTools.Select(tool => tool == "crop" ? "region" : tool).ToHashSet(StringComparer.Ordinal);
        }
        menuTheme = new NativeMenuTheme(dark);
        var english = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase);
        string T(string chinese, string translated) => english ? translated : chinese;
        AutoScaleMode = AutoScaleMode.Dpi;
        BackColor = dark ? Color.FromArgb(23, 23, 23) : Color.White;
        ForeColor = dark ? Color.FromArgb(250, 250, 250) : Color.FromArgb(23, 23, 23);
        foreach (var row in new ToolStrip[] { actions, tools, grip }) {
            row.BackColor = BackColor; row.ForeColor = ForeColor;
            row.Renderer = new OverflowRenderer(new NativeMenuColors(borderless: true, dark: dark), () => overflowImage, menuTheme.DrawArrow) { RoundedEdges = false };
            var previousMenu = row.OverflowButton.DropDown;
            var menu = new ToolStripDropDownMenu { BackColor = BackColor, ForeColor = ForeColor, Renderer = row.Renderer,
                Padding = new Padding(4), MinimumSize = new Size(160, 0), ImageScalingSize = new Size(18, 18) };
            row.OverflowButton.DropDown = menu; previousMenu.Dispose();
            menuTheme.Apply(menu);
            row.OverflowButton.DropDownOpening += (_, _) => {
                var current = row.OverflowButton.DropDown;
                // Keep customization choices and their unsaved state in this
                // very menu; rebuild only overflow command mirrors.
                foreach (var old in current.Items.Cast<ToolStripItem>().Where(item => item.Tag is ToolStripButton).ToArray()) old.Dispose();
                var index = 0;
                foreach (var original in row.Items.OfType<ToolStripButton>().Where(item => item.Placement == ToolStripItemPlacement.Overflow)) {
                    if (original.Name == "customize") continue;
                    var entry = new ToolStripMenuItem(original.Text, original.Image) { Enabled = original.Enabled, Checked = original.Checked,
                        Padding = new Padding(8, 6, 8, 6), ToolTipText = original.ToolTipText, Name = original.Name, Tag = original };
                    entry.Click += (_, _) => original.PerformClick(); current.Items.Insert(index++, entry);
                }
                menuTheme.Apply(current);
            };
            row.OverflowButton.AutoSize = false;
            row.OverflowButton.Size = new Size(32, 36);
            row.OverflowButton.ToolTipText = T("更多操作", "More actions");
            row.OverflowButton.AccessibleName = T("更多操作", "More actions");
        }
        Font = menuTheme.Font;
        Padding = new Padding(8);
        actions.Name = "capture-actions"; tools.Name = "capture-tools"; grip.Name = "capture-grip";
        Controls.Add(actions); Controls.Add(tools); Controls.Add(grip);
        grip.GripStyle = ToolStripGripStyle.Hidden;
        grip.Items.Add(gripIcon);
        grip.Renderer = new ToolStripProfessionalRenderer(new NativeMenuColors(borderless: true, dark: dark)) { RoundedEdges = false };
        grip.CanOverflow = false; grip.DedicatedGrip = true; grip.Cursor = Cursors.SizeAll;
        grip.AccessibleName = T("拖动工具栏", "Drag toolbar");
        gripHint.SetToolTip(grip, T("按住拖动工具栏", "Hold to drag toolbar"));
        grip.GripMoveRequested += point => MoveRequested?.Invoke(point);
        actions.AccessibleName = T("文件与完成操作", "File and completion actions");
        tools.AccessibleName = T("标注与文字处理", "Annotation and text tools");
        // Keep a native divider at the strip boundary even when file actions
        // overflow; confirmation and cancellation remain a distinct group.
        // The preceding strip already reserves its native trailing inset;
        // use a narrower divider here to match the other groups' visible gap.
        Separate(actions, ToolStripItemOverflow.Never, 18);
        Add(actions, "undo", T("撤销", "Undo"), T("撤销 · Ctrl+Z", "Undo · Ctrl+Z"));
        Add(actions, "redo", T("重做", "Redo"), T("重做 · Ctrl+Shift+Z", "Redo · Ctrl+Shift+Z"));
        Separate(actions);
        Add(actions, "save", T("保存", "Save"), T("保存 · Ctrl+S", "Save · Ctrl+S"));
        Add(actions, "saveAs", T("另存为", "Save as"), T("另存为 · Ctrl+Shift+S", "Save as · Ctrl+Shift+S"));
        Add(actions, "pin", T("截图置顶", "Pin screenshot"), T("截图置顶", "Pin screenshot"));
        Add(actions, "retake", T("重新滚动截图", "Retake scrolling capture"), T("重新选择窗口并滚动截图", "Choose a window and capture it again"));
        buttons["retake"].Available = false;
        Separate(actions);
        Add(actions, "confirm", T("确认", "Confirm"), T("确认并复制 · Enter", "Confirm and copy · Enter"));
        Add(actions, "cancel", T("取消", "Cancel"), T("取消 · Esc", "Cancel · Esc"));
        // Completion stays on the right even when less frequent file actions
        // move into the native overflow menu. Labels remain in tooltips/UIA.
        foreach (var command in new[] { "confirm", "cancel" }) buttons[command].Overflow = ToolStripItemOverflow.Never;
        Add(tools, "select", T("移动", "Move"), T("拖动单个标注", "Move an annotation"));
        Add(tools, "region", T("选区", "Selection"), T("调整截图范围", "Adjust capture area"));
        // Selection and scrolling both change the captured area. Keep them
        // adjacent in the native toolbar rather than among file actions.
        Add(tools, "scroll", T("滚动截图", "Scrolling capture"), T("直接滚动捕获当前选区", "Capture scrolling content in the current selection"));
        // This capture action is fixed, not a configurable annotation tool.
        drawingCommands.Remove("scroll");
        buttons["scroll"].Enabled=false;
        Add(tools, "video", T("录制视频", "Record video"), T("录制当前屏幕选区为 MP4", "Record the selected screen area as MP4"));
        Add(tools, "gif", T("录制 GIF", "Record GIF"), T("录制当前屏幕选区为 GIF", "Record the selected screen area as GIF"));
        foreach (var command in new[] { "video", "gif" }) { drawingCommands.Remove(command); buttons[command].Enabled = false; }
        Separate(tools);
        Add(tools, "rectangle", T("矩形", "Rectangle"), T("矩形", "Rectangle"));
        Add(tools, "ellipse", T("椭圆", "Ellipse"), T("椭圆", "Ellipse"));
        Add(tools, "arrow", T("箭头", "Arrow"), T("箭头", "Arrow"));
        Add(tools, "pen", T("画笔", "Pen"), T("画笔", "Pen"));
        Add(tools, "text", T("文字", "Text"), T("文字 · T", "Text · T"));
        Separate(tools);
        Add(tools, "cover", T("实色遮盖", "Cover"), T("实色遮盖", "Cover"));
        Add(tools, "mosaic", T("马赛克", "Pixelate"), T("马赛克", "Pixelate"));
        Add(tools, "blur", T("模糊", "Blur"), T("模糊", "Blur"));
        options = new NativeToolOptions(dark, language);
        menuTheme.Apply(options.DropDown);
        options.StyleChanged += style => ToolStyleChanged?.Invoke(style);
        tools.Items.Add(options);
        Separate(tools);
        Add(tools, "ocr", T("OCR", "OCR"), T("识别选区文字", "Recognize selected text"));
        Add(tools, "translate", T("翻译", "Translate"), T("识别并翻译选区文字", "Recognize and translate selected text"));
        if (saveToolbar is not null) {
            customization = new NativeToolbarCustomization(buttons.Where(pair => drawingCommands.Contains(pair.Key))
                .Select(pair => (pair.Key == "region" ? "crop" : pair.Key, pair.Value.Text!)), toolbarTools?.ToArray(), english);
            customization.Submitted += SaveToolbarPreferences;
            var previous = tools.OverflowButton.DropDown;
            tools.OverflowButton.DropDown = customization; previous.Dispose();
            menuTheme.Apply(customization);
            tools.OverflowButton.ToolTipText = customization.Text;
            tools.OverflowButton.AccessibleName = customization.Text;
            tools.Items.Add(new ToolStripButton(customization.Text) { Name = "customize", Overflow = ToolStripItemOverflow.Always });
        }
        Height = 52;
        Width = 600;
        LoadIcons();
    }

    public void SetScrollState(bool available, bool enabled) { scrollAvailable=available;scrollEnabled=enabled; buttons["scroll"].Enabled=available;buttons["scroll"].Checked=enabled; }

    public Size PreferredToolbarSize => new(
        RowWidth(actions) + RowWidth(tools) + Padding.Horizontal + GripColumnWidth,
        Math.Max(actions.Height, tools.Height) + Padding.Vertical);
    private static int RowWidth(ToolStrip row) => row.Items.Cast<ToolStripItem>().Where(item => item.Available && item.Overflow != ToolStripItemOverflow.Always)
        .Sum(ItemWidth) + row.Padding.Horizontal
        + (row.Items.Cast<ToolStripItem>().Any(item => item.Overflow == ToolStripItemOverflow.Always) ? row.OverflowButton.Width : 12);
    // Native separators can report an owner-dependent preferred width even
    // when AutoSize is false. Respect explicit button/separator dimensions.
    private static int ItemWidth(ToolStripItem item) => (item.AutoSize ? item.GetPreferredSize(Size.Empty).Width : item.Width) + item.Margin.Horizontal;
    private async void SaveToolbarPreferences(string[]? desired)
    {
        if (preferencesBusy || saveToolbar is null || customization is null) return;
        preferencesBusy = true; customization.Enabled = false;
        try {
            PreferencesSaving?.Invoke();
            var saved = await saveToolbar(desired);
            if (IsDisposed) return;
            preferredTools = saved?.Select(tool => tool == "crop" ? "region" : tool).ToHashSet(StringComparer.Ordinal);
            customization.Confirm(saved); PreferencesChanged?.Invoke();
        } catch (Exception error) {
            Console.Error.WriteLine("Toolbar preferences: " + error.Message);
            if (!IsDisposed) PreferencesFailed?.Invoke(error.Message);
        } finally { preferencesBusy = false; if (!IsDisposed) customization.Enabled = true; }
    }
    private int GripColumnWidth => (int)Math.Round(32d * DeviceDpi / 96);

    protected override void OnPaint(PaintEventArgs e)
    {
        base.OnPaint(e);
        // Keep the floating toolbar distinct from a white capture without
        // adding a shadow; native child controls remain inside the padding.
        var border = SystemInformation.HighContrast ? SystemColors.WindowFrame
            : borderColor;
        ControlPaint.DrawBorder(e.Graphics, ClientRectangle, border, ButtonBorderStyle.Solid);
    }

    protected override void OnLayout(LayoutEventArgs e)
    {
        base.OnLayout(e);
        if (actions is null || tools is null || grip is null) return;
        // Use the existing native strips on one horizontal baseline. Reserve
        // the finish/exit pair first; annotation tools receive the remaining
        // width and both strips retain their native overflow behavior.
        var gap = (int)Math.Round(8d * DeviceDpi / 96);
        var left = Padding.Left + GripColumnWidth;
        var width = Math.Max(1, ClientSize.Width - left - Padding.Right);
        var minimumActions = actions.Items.Cast<ToolStripItem>()
            .Where(item => item.Available && item.Overflow == ToolStripItemOverflow.Never)
            .Sum(ItemWidth) + actions.OverflowButton.Width;
        var actionWidth = Math.Min(RowWidth(actions), Math.Max(minimumActions, width - RowWidth(tools)));
        var toolWidth = Math.Max(1, width - actionWidth);
        tools.Bounds = new Rectangle(left, Padding.Top, toolWidth, tools.Height);
        actions.Bounds = new Rectangle(tools.Right, Padding.Top, actionWidth, actions.Height);
        grip.Bounds = new Rectangle(Padding.Left, (ClientSize.Height - grip.Height) / 2, Math.Max(1, GripColumnWidth - gap), grip.Height);
    }

    private void LoadIcons()
    {
        var pixels = Math.Max(18, (int)Math.Ceiling(18d * DeviceDpi / 96));
        var resourceSize = new[] { 18, 24, 36, 48, 72 }.FirstOrDefault(size => size >= pixels, 72);
        actions.ImageScalingSize = tools.ImageScalingSize = new Size(pixels, pixels);
        foreach (var pair in buttons) {
            var iconName = pair.Key == "retake" ? "redo" : pair.Key;
            using var stream = typeof(NativeToolbarControl).Assembly.GetManifestResourceStream($"WinOne.Capture.Icons.{iconName}{(dark ? "-dark" : "")}-{resourceSize}.png")
                ?? throw new InvalidOperationException("Missing local capture icon: " + pair.Key);
            using var image = Image.FromStream(stream);
            var previous = pair.Value.Image;
            pair.Value.Image = new Bitmap(image); previous?.Dispose();
            pair.Value.Size = new Size((int)Math.Round(36d * DeviceDpi / 96), (int)Math.Round(36d * DeviceDpi / 96));
        }
        using var gripStream = typeof(NativeToolbarControl).Assembly.GetManifestResourceStream($"WinOne.Capture.Icons.grip{(dark ? "-dark" : "")}-{resourceSize}.png")
            ?? throw new InvalidOperationException("Missing local drag icon");
        using var gripImage = Image.FromStream(gripStream);
        var oldGrip = gripIcon.Image; gripIcon.Image = new Bitmap(gripImage); oldGrip?.Dispose();
        grip.ImageScalingSize = new Size(pixels, pixels);
        using var overflowStream = typeof(NativeToolbarControl).Assembly.GetManifestResourceStream($"WinOne.Capture.Icons.overflow{(dark ? "-dark" : "")}-{resourceSize}.png")
            ?? throw new InvalidOperationException("Missing local overflow icon");
        using var overflow = Image.FromStream(overflowStream);
        var oldOverflow = overflowImage; overflowImage = new Bitmap(overflow); oldOverflow?.Dispose();
        foreach (var row in new[] { actions, tools }) row.OverflowButton.Width = (int)Math.Round(32d * DeviceDpi / 96);
    }
    protected override void OnDpiChangedAfterParent(EventArgs e) { base.OnDpiChangedAfterParent(e); LoadIcons(); }
    protected override void Dispose(bool disposing)
    {
        if (disposing) {
            customization?.Dispose();
            gripHint.Dispose();
            overflowImage?.Dispose(); overflowImage = null;
            var dragImage = gripIcon.Image; gripIcon.Image = null; dragImage?.Dispose();
            foreach (var button in buttons.Values) { var image = button.Image; button.Image = null; image?.Dispose(); }
        }
        base.Dispose(disposing);
        if (disposing) menuTheme.Dispose();
    }

    private static GripToolStrip Row() => new() {
        AutoSize = false, Height = 36, GripStyle = ToolStripGripStyle.Hidden,
        BackColor = Color.White, ForeColor = Color.FromArgb(23, 23, 23),
        Padding = Padding.Empty, ImageScalingSize = new Size(18, 18),
        LayoutStyle = ToolStripLayoutStyle.HorizontalStackWithOverflow,
        CanOverflow = true, ShowItemToolTips = true,
    };

    private void Add(ToolStrip row, string id, string label, string hint)
    {
        var button = new ToolStripButton(label) {
            Name = id, AccessibleName = label, ToolTipText = hint,
            DisplayStyle = ToolStripItemDisplayStyle.Image, AutoSize = false, Size = new Size(36, 36),
            Padding = new Padding(7), Margin = new Padding(2, 0, 2, 0),
            AutoToolTip = false, Overflow = ToolStripItemOverflow.AsNeeded,
        };
        button.Click += (_, _) => Command?.Invoke(id);
        buttons.Add(id, button); row.Items.Add(button);
        if (ReferenceEquals(row, tools)) drawingCommands.Add(id);
    }

    private static void Separate(ToolStrip row, ToolStripItemOverflow overflow = ToolStripItemOverflow.AsNeeded, int width = 28) => row.Items.Add(new ToolStripSeparator {
        AutoSize = false, Size = new Size(width, 24), Margin = new Padding(0, 6, 0, 6), Overflow = overflow
    });

    public void SetState(bool canUndo, bool canRedo, string selectedTool, bool copied, bool editing = false)
    {
        foreach (var pair in buttons) {
            if (drawingCommands.Contains(pair.Key)) {
                // Keep the current tool discoverable even when the user usually
                // stores it in the menu. This never changes the saved preference.
                // Only the selected tool is fixed. Pinning OCR and translation
                // as well can exceed a narrow row alongside Style, causing
                // ToolStrip to drop an action instead of offering overflow.
                var overflow = pair.Key == selectedTool ? ToolStripItemOverflow.Never
                    : preferredTools is null || preferredTools.Contains(pair.Key) ? ToolStripItemOverflow.AsNeeded : ToolStripItemOverflow.Always;
                if (pair.Value.Overflow != overflow) pair.Value.Overflow = overflow;
            }
            pair.Value.Checked = pair.Key == selectedTool;
            pair.Value.Enabled = !copied || pair.Key is "confirm" or "cancel";
            if (editing && pair.Key is not ("confirm" or "save" or "cancel")) pair.Value.Enabled = false;
        }
        buttons["undo"].Enabled = canUndo && !copied && !editing;
        buttons["redo"].Enabled = canRedo && !copied && !editing;
        buttons["saveAs"].Enabled = saveAsAvailable && !copied;
        buttons["pin"].Enabled = pinAvailable && !copied;
        buttons["retake"].Enabled = retakeAvailable && !copied;
        buttons["scroll"].Enabled = scrollAvailable && !copied && !editing;
        buttons["scroll"].Checked = scrollEnabled;
        buttons["video"].Enabled = buttons["gif"].Enabled = recordingAvailable && !copied && !editing;
    }
    public void SetRecordingAvailable(bool available) { recordingAvailable = available; buttons["video"].Enabled = buttons["gif"].Enabled = available; }
    public void SetSaveAsAvailable(bool available) { saveAsAvailable = available; buttons["saveAs"].Enabled = available; }
    public void SetPinAvailable(bool available) { pinAvailable = available; buttons["pin"].Enabled = available; }
    public void SetRetakeAvailable(bool available) { retakeAvailable = available; buttons["retake"].Available = available; buttons["retake"].Enabled = available; }
    public void SetToolStyle(string tool, ShareX.ScreenCaptureLib.WinOneToolStyle style, bool editing) => options.Configure(tool, style, editing);

    public object[] DescribeButtons() => buttons.Where(pair => pair.Value.Available).Select(pair => {
        var item = pair.Value;
        var parent = item.GetCurrentParent();
        return (object)new { command = pair.Key, window = parent?.IsHandleCreated == true ? parent.Handle.ToInt64().ToString() : "0",
            x = item.Bounds.X + item.Bounds.Width / 2, y = item.Bounds.Y + item.Bounds.Height / 2, enabled = item.Enabled };
    }).ToArray();
    public object DescribeGrip() => new { window = grip.Handle.ToInt64().ToString(),
        x = grip.ClientSize.Width / 2, y = grip.ClientSize.Height / 2 };

    private sealed class GripToolStrip : ToolStrip
    {
        private Point? dragPointer;
        private Point dragOrigin;
        public event Action<Point>? GripMoveRequested;
        [System.ComponentModel.DefaultValue(false)]
        internal bool DedicatedGrip { get; set; }
        // ToolStrip dispatches grip input to its internal item rather than
        // raising MouseDown/Move. Intercept only the grip before that dispatch.
        protected override void OnMouseDown(MouseEventArgs mouse)
        {
            if ((DedicatedGrip || GripStyle == ToolStripGripStyle.Visible) && mouse.Button == MouseButtons.Left && (DedicatedGrip ? ClientRectangle : GripRectangle).Contains(mouse.Location)) {
                dragPointer = PointToScreen(mouse.Location);
                dragOrigin = Parent!.PointToScreen(Point.Empty); Capture = true; return;
            }
            base.OnMouseDown(mouse);
        }
        protected override void OnMouseMove(MouseEventArgs mouse)
        {
            if (dragPointer is Point origin) {
                var pointer = PointToScreen(mouse.Location);
                GripMoveRequested?.Invoke(new Point(dragOrigin.X + pointer.X - origin.X, dragOrigin.Y + pointer.Y - origin.Y)); return;
            }
            base.OnMouseMove(mouse);
        }
        protected override void OnMouseUp(MouseEventArgs mouse)
        {
            if (dragPointer is not null) { dragPointer = null; Capture = false; return; }
            base.OnMouseUp(mouse);
        }
        protected override void OnMouseCaptureChanged(EventArgs e) { if (!Capture) dragPointer = null; base.OnMouseCaptureChanged(e); }
    }

    // Change only the native overflow button's presentation; ToolStrip keeps
    // its hit testing, keyboard navigation, menu lifetime and screen placement.
    private sealed class OverflowRenderer(ProfessionalColorTable colors, Func<Image?> icon, Action<ToolStripArrowRenderEventArgs> drawArrow) : ToolStripProfessionalRenderer(colors)
    {
        protected override void OnRenderArrow(ToolStripArrowRenderEventArgs e) => drawArrow(e);
        protected override void OnRenderOverflowButtonBackground(ToolStripItemRenderEventArgs e)
        {
            base.OnRenderButtonBackground(e);
            var image = icon(); if (image is null) return;
            var size = e.ToolStrip?.ImageScalingSize ?? new Size(18, 18);
            e.Graphics.DrawImage(image, new Rectangle((e.Item.Width - size.Width) / 2, (e.Item.Height - size.Height) / 2, size.Width, size.Height));
        }
    }

}
