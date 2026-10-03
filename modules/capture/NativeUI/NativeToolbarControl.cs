namespace WinOne.Capture.NativeUI;

// Native presentation only: commands are handled by the capture session.
// Standard ToolStrip controls own focus, accessibility and button interaction.
internal sealed class NativeToolbarControl : UserControl
{
    private readonly GripToolStrip actions = Row();
    private readonly ToolStrip tools = Row();
    private readonly GripToolStrip grip = Row();
    private readonly ToolTip gripHint = new();
    private readonly ToolStripLabel gripIcon = new() { DisplayStyle = ToolStripItemDisplayStyle.Image, AutoSize = false, Size = new Size(24, 36), Margin = Padding.Empty, ImageAlign = ContentAlignment.MiddleCenter };
    private bool saveAsAvailable = true;
    private bool pinAvailable = true;
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
            var allowed = new HashSet<string>(new[] { "select", "crop", "rectangle", "ellipse", "arrow", "pen", "cover", "text", "mosaic", "blur" }, StringComparer.Ordinal);
            if (toolbarTools.Any(tool => !allowed.Remove(tool))) throw new ArgumentException("Invalid toolbar preferences", nameof(toolbarTools));
            // Legacy move is now a direct canvas gesture; crop is the native
            // region tool. Preferences stay compatible with Host/profile data.
            preferredTools = toolbarTools.Where(tool => tool != "select").Select(tool => tool == "crop" ? "region" : tool).ToHashSet(StringComparer.Ordinal);
        }
        menuTheme = new NativeMenuTheme(dark);
        var english = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase);
        string T(string chinese, string translated) => english ? translated : chinese;
        AutoScaleMode = AutoScaleMode.Dpi;
        BackColor = dark ? Color.FromArgb(23, 23, 23) : Color.White;
        ForeColor = dark ? Color.FromArgb(250, 250, 250) : Color.FromArgb(23, 23, 23);
        foreach (var row in new ToolStrip[] { actions, tools, grip }) {
            row.BackColor = BackColor; row.ForeColor = ForeColor;
            row.Renderer = new OverflowRenderer(new NativeMenuColors(dark: dark), () => overflowImage) { RoundedEdges = false };
            var previousMenu = row.OverflowButton.DropDown;
            var menu = new ToolStripDropDownMenu { BackColor = BackColor, ForeColor = ForeColor, Renderer = row.Renderer,
                Padding = new Padding(4), MinimumSize = new Size(160, 0), ImageScalingSize = new Size(18, 18) };
            row.OverflowButton.DropDown = menu; previousMenu.Dispose();
            menuTheme.Apply(menu);
            row.OverflowButton.DropDownOpening += (_, _) => {
                if (customization?.Owner == menu) menu.Items.Remove(customization);
                foreach (var old in menu.Items.Cast<ToolStripItem>().ToArray()) old.Dispose();
                foreach (var original in row.Items.OfType<ToolStripButton>().Where(item => item.Placement == ToolStripItemPlacement.Overflow)) {
                    if (original.Name == "customize" && customization is not null) { menu.Items.Add(customization); continue; }
                    var entry = new ToolStripMenuItem(original.Text, original.Image) { Enabled = original.Enabled, Checked = original.Checked,
                        Padding = new Padding(8, 6, 8, 6), ToolTipText = original.ToolTipText, Name = original.Name };
                    entry.Click += (_, _) => original.PerformClick(); menu.Items.Add(entry);
                }
                menuTheme.Apply(menu);
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
        Add(actions, "confirm", T("确认", "Confirm"), T("确认并复制 · Enter", "Confirm and copy · Enter"));
        Add(actions, "save", T("保存", "Save"), T("保存 · Ctrl+S", "Save · Ctrl+S"));
        Add(actions, "saveAs", T("另存为", "Save as"), T("另存为 · Ctrl+Shift+S", "Save as · Ctrl+Shift+S"));
        Add(actions, "pin", T("截图置顶", "Pin screenshot"), T("截图置顶", "Pin screenshot"));
        Add(actions, "undo", T("撤销", "Undo"), T("撤销 · Ctrl+Z", "Undo · Ctrl+Z"));
        Add(actions, "redo", T("重做", "Redo"), T("重做 · Ctrl+Shift+Z", "Redo · Ctrl+Shift+Z"));
        Add(actions, "cancel", T("取消", "Cancel"), T("取消 · Esc", "Cancel · Esc"));
        // Reserve the session's finish/exit pair. Keeping a third text button
        // fixed clips controls at narrow widths in longer translations.
        foreach (var command in new[] { "confirm", "cancel" }) buttons[command].Overflow = ToolStripItemOverflow.Never;
        Add(tools, "region", T("选区", "Selection"), T("调整截图范围", "Adjust capture area"));
        Add(tools, "rectangle", T("矩形", "Rectangle"), T("矩形", "Rectangle"));
        Add(tools, "ellipse", T("椭圆", "Ellipse"), T("椭圆", "Ellipse"));
        Add(tools, "arrow", T("箭头", "Arrow"), T("箭头", "Arrow"));
        Add(tools, "pen", T("画笔", "Pen"), T("画笔", "Pen"));
        Add(tools, "cover", T("实色遮盖", "Cover"), T("实色遮盖", "Cover"));
        Add(tools, "mosaic", T("马赛克", "Pixelate"), T("马赛克", "Pixelate"));
        Add(tools, "blur", T("模糊", "Blur"), T("模糊", "Blur"));
        Add(tools, "text", T("文字", "Text"), T("文字 · T", "Text · T"));
        options = new NativeToolOptions(dark, language);
        menuTheme.Apply(options.DropDown);
        options.StyleChanged += style => ToolStyleChanged?.Invoke(style);
        tools.Items.Add(options);
        if (saveToolbar is not null) {
            customization = new NativeToolbarCustomization(buttons.Where(pair => drawingCommands.Contains(pair.Key))
                .Select(pair => (pair.Key == "region" ? "crop" : pair.Key, pair.Value.Text!)), toolbarTools?.ToArray(), english);
            customization.Submitted += SaveToolbarPreferences;
            menuTheme.Apply(customization.DropDown);
            tools.Items.Add(new ToolStripButton(customization.Text) { Name = "customize", Overflow = ToolStripItemOverflow.Always });
        }
        Height = 96;
        Width = 600;
        LoadIcons();
    }

    public Size PreferredToolbarSize => new(
        Math.Max(RowWidth(actions), RowWidth(tools)) + Padding.Horizontal + GripColumnWidth,
        actions.Height + tools.Height + Padding.Vertical + 8);
    private static int RowWidth(ToolStrip row) => row.Items.Cast<ToolStripItem>().Where(item => item.Available && item.Overflow != ToolStripItemOverflow.Always)
        .Sum(item => item.GetPreferredSize(Size.Empty).Width + item.Margin.Horizontal) + row.Padding.Horizontal
        + (row.Items.Cast<ToolStripItem>().Any(item => item.Overflow == ToolStripItemOverflow.Always) ? row.OverflowButton.Width : 12);
    private async void SaveToolbarPreferences(string[]? desired)
    {
        if (preferencesBusy || saveToolbar is null || customization is null) return;
        preferencesBusy = true; customization.Enabled = false;
        try {
            PreferencesSaving?.Invoke();
            var saved = await saveToolbar(desired);
            if (IsDisposed) return;
            preferredTools = saved?.Where(tool => tool != "select").Select(tool => tool == "crop" ? "region" : tool).ToHashSet(StringComparer.Ordinal);
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
        // One native grip belongs to the complete toolbar, not its first row.
        var gap = (int)Math.Round(8d * DeviceDpi / 96);
        var left = Padding.Left + GripColumnWidth;
        var width = Math.Max(1, ClientSize.Width - left - Padding.Right);
        actions.Bounds = new Rectangle(left, Padding.Top, width, actions.Height);
        tools.Bounds = new Rectangle(left, ClientSize.Height - Padding.Bottom - tools.Height, width, tools.Height);
        grip.Bounds = new Rectangle(Padding.Left, (ClientSize.Height - grip.Height) / 2, Math.Max(1, GripColumnWidth - gap), grip.Height);
    }

    private void LoadIcons()
    {
        var pixels = Math.Max(18, (int)Math.Ceiling(18d * DeviceDpi / 96));
        var resourceSize = new[] { 18, 24, 36, 48, 72 }.FirstOrDefault(size => size >= pixels, 72);
        actions.ImageScalingSize = tools.ImageScalingSize = new Size(pixels, pixels);
        foreach (var pair in buttons) {
            using var stream = typeof(NativeToolbarControl).Assembly.GetManifestResourceStream($"WinOne.Capture.Icons.{pair.Key}{(dark ? "-dark" : "")}-{resourceSize}.png")
                ?? throw new InvalidOperationException("Missing local capture icon: " + pair.Key);
            using var image = Image.FromStream(stream);
            var previous = pair.Value.Image;
            pair.Value.Image = new Bitmap(image); previous?.Dispose();
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
            DisplayStyle = ToolStripItemDisplayStyle.ImageAndText,
            Padding = new Padding(8, 4, 8, 4), Margin = new Padding(2, 0, 2, 0),
            AutoToolTip = false, Overflow = ToolStripItemOverflow.AsNeeded,
        };
        button.Click += (_, _) => Command?.Invoke(id);
        buttons.Add(id, button); row.Items.Add(button);
        if (ReferenceEquals(row, tools)) drawingCommands.Add(id);
    }

    public void SetState(bool canUndo, bool canRedo, string selectedTool, bool copied, bool editing = false)
    {
        foreach (var pair in buttons) {
            if (drawingCommands.Contains(pair.Key)) {
                // Keep the current tool discoverable even when the user usually
                // stores it in the menu. This never changes the saved preference.
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
    }
    public void SetSaveAsAvailable(bool available) { saveAsAvailable = available; buttons["saveAs"].Enabled = available; }
    public void SetPinAvailable(bool available) { pinAvailable = available; buttons["pin"].Enabled = available; }
    public void SetToolStyle(string tool, ShareX.ScreenCaptureLib.WinOneToolStyle style, bool editing) => options.Configure(tool, style, editing);

    public object[] DescribeButtons() => buttons.Select(pair => {
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
    private sealed class OverflowRenderer(ProfessionalColorTable colors, Func<Image?> icon) : ToolStripProfessionalRenderer(colors)
    {
        protected override void OnRenderOverflowButtonBackground(ToolStripItemRenderEventArgs e)
        {
            base.OnRenderButtonBackground(e);
            var image = icon(); if (image is null) return;
            var size = e.ToolStrip?.ImageScalingSize ?? new Size(18, 18);
            e.Graphics.DrawImage(image, new Rectangle((e.Item.Width - size.Width) / 2, (e.Item.Height - size.Height) / 2, size.Width, size.Height));
        }
    }

}
