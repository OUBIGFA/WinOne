using ShareX.HelpersLib;
using WinOne.Capture.NativeUI;

namespace ShareX;

// WinOne owns the controls; the upstream form continues handling painting,
// dragging, keyboard movement and the bounded scale/opacity values.
public partial class PinToScreenForm
{
    private bool managedMenu;
    private bool pinEnglish;
    private string? managedFileNameTemplate;
    private string PinText(string zh, string en) => pinEnglish ? en : zh;

    private void ConfigureWinOneMenu(string? language)
    {
        managedMenu = true;
        pinEnglish = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase);
        Text = PinText("WinOne · 置顶截图", "WinOne · Pinned image");
        var dark = ShareXResources.Theme.IsDarkTheme;
        var theme = new NativeMenuTheme(dark);
        var panel = theme.Background; var text = theme.Foreground;
        Disposed += (_, _) => theme.Dispose();
        components ??= new System.ComponentModel.Container();
        var menu = new ContextMenuStrip(components) { ShowImageMargin = false, ShowCheckMargin = true };
        ToolStripMenuItem Add(ToolStripItemCollection items, string name, string label, Action? action = null)
        {
            var item = new ToolStripMenuItem(label) { Name = name, ForeColor = text, BackColor = panel };
            if (action is not null) item.Click += (_, _) => action();
            items.Add(item); return item;
        }
        Add(menu.Items, "pin-copy", PinText("复制", "Copy"), CopyManagedImage).ShortcutKeyDisplayString = "Ctrl+C";
        Add(menu.Items, "pin-save", PinText("另存原图", "Save original as"), SaveManagedImage);
        Add(menu.Items, "pin-reset", PinText("恢复大小与透明度", "Reset size and opacity"), () => { if (Minimized) ToggleMinimize(); ResetImage(); });
        var collapse = Add(menu.Items, "pin-collapse", PinText("折叠", "Collapse"), ToggleMinimize);
        menu.Items.Add(new ToolStripSeparator());
        var view = Add(menu.Items, "pin-view", PinText("显示", "View"));
        var scales = Add(view.DropDownItems, "pin-scale", PinText("缩放", "Zoom"));
        foreach (var scale in new[] { 50, 100, 150, 200 })
            Add(scales.DropDownItems, $"pin-scale-{scale}", $"{scale}%", () => { if (Minimized) ToggleMinimize(); ImageScale = scale; });
        var opacity = Add(view.DropDownItems, "pin-opacity", PinText("不透明度", "Opacity"));
        foreach (var value in new[] { 100, 75, 50, 25 })
            Add(opacity.DropDownItems, $"pin-opacity-{value}", $"{value}%", () => { if (Minimized) ToggleMinimize(); ImageOpacity = value; });
        var top = Add(view.DropDownItems, "pin-topmost", PinText("始终置顶", "Always on top"), () => { Options.TopMost = !Options.TopMost; TopMost = Options.TopMost; });
        var border = Add(view.DropDownItems, "pin-border", PinText("边框", "Border"), () => { Options.Border = !Options.Border; AutoSizeForm(); });
        var quality = Add(view.DropDownItems, "pin-quality", PinText("平滑缩放", "Smooth scaling"), () => { Options.HighQualityScale = !Options.HighQualityScale; Invalidate(); });
        menu.Items.Add(new ToolStripSeparator());
        Add(menu.Items, "pin-close", PinText("关闭", "Close"), Close).ShortcutKeyDisplayString = "Esc";
        menu.Opening += (_, _) =>
        {
            collapse.Text = Minimized ? PinText("展开", "Expand") : PinText("折叠", "Collapse");
            top.Checked = Options.TopMost; border.Checked = Options.Border; quality.Checked = Options.HighQualityScale;
            foreach (ToolStripMenuItem item in scales.DropDownItems) item.Checked = item.Text == $"{ImageScale}%";
            foreach (ToolStripMenuItem item in opacity.DropDownItems) item.Checked = item.Text == $"{ImageOpacity}%";
        };
        theme.Apply(menu);
        ContextMenuStrip = menu;
        // Right-click opens the context menu instead of destroying the image.
        MouseUp -= PinToScreenForm_MouseUp;
        MouseUp += (sender, args) => { if (args.Button != MouseButtons.Right) PinToScreenForm_MouseUp(sender, args); };
        KeyDown += (_, args) => { if (args.KeyCode == Keys.Escape) { args.SuppressKeyPress = true; Close(); } };
        tsbCopy.Click -= tsbCopy_Click;
        tsbCopy.Click += (_, _) => CopyManagedImage();
        tsbOptions.Click -= tsbOptions_Click;
        tsbOptions.Click += (_, _) => menu.Show(tsMain, new Point(0, tsMain.Height));
        theme.Apply(tsMain);
        tsbCopy.Text = PinText("复制", "Copy"); tsbOptions.Text = PinText("更多", "More"); tsbClose.Text = PinText("关闭", "Close");
        foreach (var item in new[] { tsbCopy, tsbOptions, tsbClose })
        {
            item.DisplayStyle = ToolStripItemDisplayStyle.Text; item.ToolTipText = item.Text; item.AccessibleName = item.Text; item.ForeColor = text;
        }
        tslScale.ToolTipText = PinText("恢复原始大小", "Restore original size");
        UpdateControls();
    }

    private void CopyManagedImage()
    {
        if (!ClipboardHelpers.CopyImage(Image))
            MessageBox.Show(this, PinText("剪贴板暂不可用，请重试。", "Clipboard unavailable. Try again."), "WinOne", MessageBoxButtons.OK, MessageBoxIcon.Warning);
    }

    private void SaveManagedImage()
    {
        using var dialog = new SaveFileDialog { Title = PinText("置顶截图另存为", "Save pinned image as"), Filter = "PNG|*.png", DefaultExt = "png", FileName = WinOne.Capture.PngFiles.SuggestedName(DateTime.Now, template: managedFileNameTemplate), OverwritePrompt = true };
        if (dialog.ShowDialog(this) != DialogResult.OK) return;
        try { WinOne.Capture.PngFiles.Write(Image, dialog.FileName); }
        catch (Exception error)
        {
            Console.Error.WriteLine(error);
            MessageBox.Show(this, PinText("保存失败，请选择其他位置重试。", "Save failed. Choose another location and try again."), "WinOne", MessageBoxButtons.OK, MessageBoxIcon.Warning);
        }
    }

}
