namespace WinOne.Capture.NativeUI;

internal sealed class NativeMenuColors : ProfessionalColorTable
    {
        private readonly bool borderless;
        private readonly bool dark;
        private Color Background => dark ? Color.FromArgb(23, 23, 23) : Color.White;
        public NativeMenuColors(bool borderless = false, bool dark = false) { this.borderless = borderless; this.dark = dark; UseSystemColors = false; }
        public override Color ToolStripGradientBegin => Background;
        public override Color ToolStripGradientMiddle => Background;
        public override Color ToolStripGradientEnd => Background;
        public override Color ToolStripBorder => borderless ? Background : dark ? Color.FromArgb(64, 64, 64) : Color.FromArgb(229, 229, 229);
        public override Color ButtonSelectedHighlight => dark ? Color.FromArgb(38, 38, 38) : Color.FromArgb(245, 245, 245);
        public override Color ButtonSelectedGradientBegin => ButtonSelectedHighlight;
        public override Color ButtonSelectedGradientMiddle => ButtonSelectedHighlight;
        public override Color ButtonSelectedGradientEnd => ButtonSelectedHighlight;
        public override Color ButtonPressedGradientBegin => dark ? Color.FromArgb(64, 64, 64) : Color.FromArgb(229, 229, 229);
        public override Color ButtonPressedGradientMiddle => ButtonPressedGradientBegin;
        public override Color ButtonPressedGradientEnd => ButtonPressedGradientBegin;
        public override Color ButtonCheckedGradientBegin => ButtonPressedGradientBegin;
        public override Color ButtonCheckedGradientMiddle => ButtonCheckedGradientBegin;
        public override Color ButtonCheckedGradientEnd => ButtonCheckedGradientBegin;
        public override Color ButtonSelectedBorder => dark ? Color.FromArgb(115, 115, 115) : Color.FromArgb(212, 212, 212);
        public override Color ButtonCheckedHighlightBorder => ButtonSelectedBorder;
        public override Color GripDark => dark ? Color.FromArgb(181, 181, 181) : Color.FromArgb(115, 115, 115);
        public override Color GripLight => Background;
        public override Color ToolStripDropDownBackground => Background;
        public override Color ImageMarginGradientBegin => Background;
        public override Color ImageMarginGradientMiddle => Background;
        public override Color ImageMarginGradientEnd => Background;
        public override Color MenuBorder => ToolStripBorder;
        public override Color MenuItemSelected => ButtonSelectedHighlight;
        public override Color MenuItemSelectedGradientBegin => MenuItemSelected;
        public override Color MenuItemSelectedGradientEnd => MenuItemSelected;
        public override Color MenuItemPressedGradientBegin => ButtonPressedGradientBegin;
        public override Color MenuItemPressedGradientMiddle => ButtonPressedGradientBegin;
        public override Color MenuItemPressedGradientEnd => ButtonPressedGradientBegin;
        public override Color CheckBackground => ButtonPressedGradientBegin;
        public override Color CheckSelectedBackground => ButtonSelectedHighlight;
        public override Color CheckPressedBackground => ButtonPressedGradientBegin;
        // Borderless strips still need visible group dividers. The native
        // renderer draws one dark line and a background-colored highlight.
        public override Color SeparatorDark => SystemInformation.HighContrast ? SystemColors.MenuText
            : dark ? Color.FromArgb(82, 82, 82) : Color.FromArgb(205, 205, 205);
        public override Color SeparatorLight => Background;
        public override Color MenuItemBorder => ButtonSelectedBorder;
        public override Color OverflowButtonGradientBegin => Background;
        public override Color OverflowButtonGradientMiddle => Background;
        public override Color OverflowButtonGradientEnd => Background;
    }
