using System.Text.Json;
using System.Runtime.InteropServices;

internal static class NativeToolbarCustomizationProbe
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    public static int Run(string output)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private desktop");
        if (Directory.Exists(output)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        Application.SetUnhandledExceptionMode(UnhandledExceptionMode.ThrowException);
        var attempts = 0; var changed = 0; var failed = 0; var released = false;
        TaskCompletionSource<string[]?>? pending = null;
        string[]? saved = null;
        async Task<string[]?> Save(string[]? value) {
            attempts++;
            if (attempts == 1) throw new InvalidOperationException("save rejected");
            if (pending is not null) { var result = await pending.Task; released = true; return result; }
            saved = value; return value;
        }
        using var parent = new Form { ShowInTaskbar = false };
        using var toolbar = new NativeToolbarControl(dark: Environment.GetEnvironmentVariable("WINONE_PROBE_THEME") == "dark", language: Environment.GetEnvironmentVariable("WINONE_PROBE_LANGUAGE") ?? "zh-CN", saveToolbar: Save);
        toolbar.PreferencesFailed += _ => failed++;
        toolbar.PreferencesChanged += () => { changed++; toolbar.SetState(false, false, "rectangle", false); };
        parent.Controls.Add(toolbar); toolbar.SetState(false, false, "rectangle", false);
        toolbar.Size = toolbar.PreferredToolbarSize; parent.ClientSize = toolbar.Size; parent.Show();
        var tools = (ToolStrip)toolbar.Controls["capture-tools"]!;
        NativeToolbarCustomization Open() {
            tools.OverflowButton.ShowDropDown();
            var item = tools.OverflowButton.DropDown.Items.OfType<NativeToolbarCustomization>().Single(); item.ShowDropDown();
            return item;
        }
        var menu = Open();
        var choices = menu.DropDownItems.OfType<ToolStripMenuItem>().Where(item => item.Tag is string).ToArray();
        var before = choices[0].Checked; var bounds = choices[0].Bounds;
        var rootBefore = tools.OverflowButton.DropDown.Visible; var childBefore = menu.DropDown.Visible;
        var point = new IntPtr(((bounds.Top + bounds.Height / 2) << 16) | (bounds.Left + 16));
        SendMessageW(menu.DropDown.Handle, 0x0201, new IntPtr(1), point);
        SendMessageW(menu.DropDown.Handle, 0x0202, IntPtr.Zero, point);
        File.WriteAllText(Path.Combine(output, "click.json"), JsonSerializer.Serialize(new { before, after = choices[0].Checked, rootBefore, childBefore, rootAfter = tools.OverflowButton.DropDown.Visible, childAfter = menu.DropDown.Visible, bounds, choices[0].Enabled }));
        if (choices[0].Checked == before || !menu.DropDown.Visible || !tools.OverflowButton.DropDown.Visible) throw new Exception($"Checkbox click: {before}->{choices[0].Checked}, root {rootBefore}->{tools.OverflowButton.DropDown.Visible}, child {childBefore}->{menu.DropDown.Visible}");
        Application.DoEvents();
        for (var i = 0; i < choices.Length; i++) choices[i].Checked = i is 2 or 4;
        ((ToolStripMenuItem)menu.DropDownItems.Find("toolbar-save", false).Single()).PerformClick();
        if (failed != 1 || changed != 0 || choices.Count(item => item.Checked) != 2) throw new Exception("Failed save changed applied preferences or lost the draft");
        menu.HideDropDown(); tools.OverflowButton.HideDropDown(); menu = Open();
        if (choices.Count(item => item.Checked) != 2) throw new Exception("Reopening customization lost the unsaved selection");
        ((ToolStripMenuItem)menu.DropDownItems.Find("toolbar-save", false).Single()).PerformClick();
        if (changed != 1 || saved?.Length != 2) throw new Exception("Confirmed save was not applied");
        ((ToolStripMenuItem)menu.DropDownItems.Find("toolbar-reset", false).Single()).PerformClick();
        if (changed != 2 || saved is not null || choices.Count(item => item.Checked) != 9) throw new Exception("Restore defaults did not preserve null preference semantics");
        menu.DropDown.Update();
        using (var image = new Bitmap(menu.DropDown.Width, menu.DropDown.Height)) {
            menu.DropDown.DrawToBitmap(image, new Rectangle(Point.Empty, image.Size)); image.Save(Path.Combine(output, "customization.png"));
        }
        pending = new TaskCompletionSource<string[]?>(TaskCreationOptions.RunContinuationsAsynchronously);
        ((ToolStripMenuItem)menu.DropDownItems.Find("toolbar-save", false).Single()).PerformClick();
        if (menu.Enabled) throw new Exception("Saving allowed a second submission");
        parent.Close(); pending.SetResult(new[] { "arrow" });
        var deadline = DateTime.UtcNow.AddSeconds(3);
        while (!released && DateTime.UtcNow < deadline) { Application.DoEvents(); Thread.Sleep(1); }
        Application.DoEvents();
        if (!released || changed != 2) throw new Exception("Closing UI cancelled the save or updated disposed controls");
        File.WriteAllText(Path.Combine(output, "report.json"), JsonSerializer.Serialize(new { attempts, changed, failed, released }));
        Console.WriteLine("PASS: native customization draft, failed retry, reset and completion after UI close");
        return 0;
    }
}
