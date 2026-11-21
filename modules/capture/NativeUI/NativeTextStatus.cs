namespace WinOne.Capture.NativeUI;

// Feedback for the silent OCR shortcut, whose capture window is hidden while
// recognition runs. One small popup below the selection says "recognizing",
// then "copied". It never takes focus or pointer input, uses the toolbar's
// design tokens, and is closed before any new capture reads the screen.
// Failures need no popup: the capture window returns with the error.
internal static class NativeTextStatus
{
    // Results faster than this appear without a "recognizing" flash.
    internal const int RecognizingDelayMs = 150;
    internal const int CopiedMs = 1200;
    private static StatusForm? form;
    private static System.Windows.Forms.Timer? timer;

    // Observed by tests on the owning UI thread.
    internal static string? Text => form is { IsDisposed: false, Visible: true } ? form.Message : null;

    internal static void ShowRecognizing(Rectangle selection, bool dark, bool english)
    {
        Dismiss();
        var status = form = new StatusForm(selection, NativeStyle.For(dark), english ? "Recognizing…" : "正在识别…", english ? "Text copied" : "已复制识别结果");
        Arm(RecognizingDelayMs, () => status.ShowMessage(status.Recognizing));
    }

    internal static void ShowCopied()
    {
        if (form is not { IsDisposed: false } status) return;
        status.ShowMessage(status.Copied);
        Arm(CopiedMs, Dismiss);
    }

    internal static void Dismiss()
    {
        timer?.Dispose(); timer = null;
        form?.Dispose(); form = null;
    }

    private static void Arm(int milliseconds, Action action)
    {
        timer?.Dispose();
        var next = timer = new System.Windows.Forms.Timer { Interval = milliseconds };
        next.Tick += (_, _) => { next.Stop(); if (ReferenceEquals(timer, next)) action(); };
        next.Start();
    }

    private sealed class StatusForm : Form
    {
        private readonly Rectangle anchor;
        private readonly NativeStyle style;
        private readonly Font font = NativeStyle.CreateFont();
        internal string Recognizing { get; }
        internal string Copied { get; }
        internal string Message { get; private set; } = "";

        internal StatusForm(Rectangle anchor, NativeStyle style, string recognizing, string copied)
        {
            this.anchor = anchor; this.style = style; Recognizing = recognizing; Copied = copied;
            FormBorderStyle = FormBorderStyle.None; ShowInTaskbar = false; TopMost = true;
            StartPosition = FormStartPosition.Manual; BackColor = style.Panel; Font = font;
            SetStyle(ControlStyles.AllPaintingInWmPaint | ControlStyles.OptimizedDoubleBuffer | ControlStyles.UserPaint, true);
        }

        protected override bool ShowWithoutActivation => true;

        // No activation, no taskbar entry and click-through: the popup must not
        // steal focus from the app the text is pasted into.
        protected override CreateParams CreateParams
        {
            get {
                var parameters = base.CreateParams;
                parameters.ExStyle |= 0x08000000 | 0x00000080 | 0x00000008 | 0x00000020;
                return parameters;
            }
        }

        internal void ShowMessage(string message)
        {
            Message = message;
            var dpi = DeviceDpi;
            var text = TextRenderer.MeasureText(message, Font);
            var size = new Size(text.Width + NativeStyle.Scale(24, dpi), text.Height + NativeStyle.Scale(12, dpi));
            // Below the selection, centred, kept on the selection's screen.
            var area = Screen.FromRectangle(anchor).WorkingArea;
            var gap = NativeStyle.Scale(8, dpi);
            var x = Math.Clamp(anchor.Left + (anchor.Width - size.Width) / 2, area.Left, Math.Max(area.Left, area.Right - size.Width));
            var y = anchor.Bottom + gap + size.Height <= area.Bottom ? anchor.Bottom + gap : Math.Max(area.Top, anchor.Top - gap - size.Height);
            Bounds = new Rectangle(x, y, size.Width, size.Height);
            using (var path = NativeStyle.Rounded(new RectangleF(0, 0, size.Width, size.Height), NativeStyle.Scale(NativeStyle.PanelRadius, dpi)))
                Region = new Region(path);
            Invalidate();
            if (!Visible) Show();
        }

        protected override void OnPaint(PaintEventArgs args)
        {
            var radius = NativeStyle.Scale(NativeStyle.PanelRadius, DeviceDpi);
            NativeStyle.FillRounded(args.Graphics, new RectangleF(0, 0, Width, Height), radius, style.Panel);
            using (var pen = new Pen(style.Border))
            using (var path = NativeStyle.Rounded(new RectangleF(0.5f, 0.5f, Width - 1, Height - 1), radius))
                args.Graphics.DrawPath(pen, path);
            TextRenderer.DrawText(args.Graphics, Message, Font, ClientRectangle, style.Text,
                TextFormatFlags.HorizontalCenter | TextFormatFlags.VerticalCenter | TextFormatFlags.SingleLine | TextFormatFlags.NoPrefix);
        }

        protected override void Dispose(bool disposing)
        {
            base.Dispose(disposing);
            if (disposing) font.Dispose();
        }
    }
}
