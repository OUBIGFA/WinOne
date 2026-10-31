namespace WinOne.Capture.NativeUI;

// The professional renderer's colors in the settings window's palette.
// NativeRenderer draws every surface of the toolbar and its menus itself;
// this table keeps the few the base renderer still reads, and gives other
// capture panels the shared border color.
internal sealed class NativeMenuColors : ProfessionalColorTable
    {
        private readonly bool borderless;
        private readonly NativeStyle style;
        private Color Background => style.Panel;
        public NativeMenuColors(bool borderless = false, bool dark = false) : this(NativeStyle.For(dark), borderless) { }
        public NativeMenuColors(NativeStyle style, bool borderless = false) { this.borderless = borderless; this.style = style; UseSystemColors = false; }
        public override Color ToolStripGradientBegin => Background;
        public override Color ToolStripGradientMiddle => Background;
        public override Color ToolStripGradientEnd => Background;
        public override Color ToolStripBorder => borderless ? Background : style.Border;
        public override Color ButtonSelectedHighlight => style.Hover;
        public override Color ButtonSelectedGradientBegin => ButtonSelectedHighlight;
        public override Color ButtonSelectedGradientMiddle => ButtonSelectedHighlight;
        public override Color ButtonSelectedGradientEnd => ButtonSelectedHighlight;
        public override Color ButtonPressedGradientBegin => style.Pressed;
        public override Color ButtonPressedGradientMiddle => ButtonPressedGradientBegin;
        public override Color ButtonPressedGradientEnd => ButtonPressedGradientBegin;
        public override Color ButtonCheckedGradientBegin => style.Selected;
        public override Color ButtonCheckedGradientMiddle => ButtonCheckedGradientBegin;
        public override Color ButtonCheckedGradientEnd => ButtonCheckedGradientBegin;
        public override Color ButtonSelectedBorder => style.Border;
        public override Color ButtonCheckedHighlightBorder => ButtonSelectedBorder;
        public override Color GripDark => style.TextMuted;
        public override Color GripLight => Background;
        public override Color ToolStripDropDownBackground => Background;
        public override Color ImageMarginGradientBegin => Background;
        public override Color ImageMarginGradientMiddle => Background;
        public override Color ImageMarginGradientEnd => Background;
        public override Color MenuBorder => style.Border;
        public override Color MenuItemSelected => style.Hover;
        public override Color MenuItemSelectedGradientBegin => MenuItemSelected;
        public override Color MenuItemSelectedGradientEnd => MenuItemSelected;
        public override Color MenuItemPressedGradientBegin => style.Pressed;
        public override Color MenuItemPressedGradientMiddle => style.Pressed;
        public override Color MenuItemPressedGradientEnd => style.Pressed;
        public override Color CheckBackground => style.Selected;
        public override Color CheckSelectedBackground => style.Hover;
        public override Color CheckPressedBackground => style.Pressed;
        public override Color SeparatorDark => SystemInformation.HighContrast ? SystemColors.MenuText : style.Border;
        public override Color SeparatorLight => Background;
        public override Color MenuItemBorder => style.Hover;
        public override Color OverflowButtonGradientBegin => Background;
        public override Color OverflowButtonGradientMiddle => Background;
        public override Color OverflowButtonGradientEnd => Background;
    }
