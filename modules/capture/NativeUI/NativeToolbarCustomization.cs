namespace WinOne.Capture.NativeUI;

// Native checked menu items stay open for multiple selections.
// The draft remains local until Host confirms the save.
internal sealed class NativeToolbarCustomization : ToolStripDropDownMenu
{
    private readonly List<ToolStripMenuItem> choices = new();
        protected override void OnItemClicked(ToolStripItemClickedEventArgs e)
        {
            // ToolStripDropDown closes leaf items in OnItemClicked. Suspend
            // only that click's dismissal; outside clicks and Escape stay native.
            var previous = AutoClose;
            if (e.ClickedItem is ToolStripMenuItem { CheckOnClick: true }) AutoClose = false;
            try { base.OnItemClicked(e); }
            finally { AutoClose = previous; }
        }
    private bool defaults;
    private bool loading;
    public event Action<string[]?>? Submitted;

    public NativeToolbarCustomization(IEnumerable<(string Key, string Label)> choices, string[]? saved, bool english)
    {
        Name = "customize";
        Text = english ? "Customize toolbar" : "自定义工具栏";
        ShowImageMargin = false; ShowCheckMargin = true;
        Items.Add(new ToolStripLabel(Text));
        foreach (var choice in choices) {
            var item = new ToolStripMenuItem(choice.Label) { Name = "toolbar-tool-" + choice.Key, Tag = choice.Key, CheckOnClick = true };
            item.CheckedChanged += (_, _) => { if (!loading) defaults = false; };
            this.choices.Add(item); Items.Add(item);
        }
        Items.Add(new ToolStripSeparator());
        var save = new ToolStripMenuItem(english ? "Save" : "保存") { Name = "toolbar-save" };
        var reset = new ToolStripMenuItem(english ? "Restore defaults" : "恢复默认") { Name = "toolbar-reset" };
        save.Click += (_, _) => Submitted?.Invoke(defaults ? null : this.choices.Where(item => item.Checked).Select(item => (string)item.Tag!).ToArray());
        reset.Click += (_, _) => { Confirm(null); Submitted?.Invoke(null); };
        Items.Add(save); Items.Add(reset);
        Confirm(saved);
    }
    public void Confirm(string[]? saved)
    {
        loading = true;
        try {
            defaults = saved is null;
            var shown = saved?.ToHashSet(StringComparer.Ordinal);
            foreach (var item in choices) item.Checked = shown is null || shown.Contains((string)item.Tag!);
        } finally { loading = false; }
    }
}
