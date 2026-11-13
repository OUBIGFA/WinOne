using System.Diagnostics;
using System.Runtime.InteropServices;
using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;
using WinOne.Capture.NativeUI;

// The Shift+C session entry, real HWND selection and real Host OCR bridge.
// Generated pixels and an in-memory copy receiver keep the user's desktop and
// clipboard untouched. The report explicitly excludes physical hotkey/copy IO.
internal static class NativeOcrCopyLatencyProbe
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    private static IntPtr At(int x, int y) => new((y << 16) | (x & 0xffff));

    private sealed class FirstPaint(Form form, Action painted) : NativeWindow, IDisposable
    {
        private bool fired;
        internal void Attach() => AssignHandle(form.Handle);
        protected override void WndProc(ref Message message)
        {
            base.WndProc(ref message);
            if (message.Msg == 0x000f && !fired) { fired = true; painted(); }
        }
        public void Dispose() => ReleaseHandle();
    }

    private sealed class TimedService(ICaptureTextService service, Stopwatch clock) : ICaptureTextService
    {
        internal double Started { get; private set; }
        internal double Finished { get; private set; }
        public async Task<string> Recognize(Bitmap image, CancellationToken cancellation)
        {
            Started = clock.Elapsed.TotalMilliseconds;
            var text = await service.Recognize(image, cancellation).ConfigureAwait(false);
            Finished = clock.Elapsed.TotalMilliseconds;
            return text;
        }
        public Task<string> Translate(string text, CancellationToken cancellation) => throw new InvalidOperationException("OCR shortcut translated text");
    }

    internal static async Task<object> Sample(string phase, NativeHostConfiguration host)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal))
            throw new InvalidOperationException("OCR copy timing requires a private desktop");
        var bounds = CaptureHelpers.GetScreenBounds();
        using var image = new Bitmap(bounds.Width, bounds.Height);
        using (var graphics = Graphics.FromImage(image)) {
            graphics.Clear(Color.White);
            using var font = new Font("Segoe UI", 24, GraphicsUnit.Pixel);
            graphics.DrawString("Hello 123", font, Brushes.Black, new PointF(50, 48));
        }
        var clock = Stopwatch.StartNew();
        var service = new TimedService(new NativeCaptureTextService(host), clock);
        using var canvas = new WinOneCaptureCanvas(image);
        double firstPaint = 0, selectionCommitted = 0, copied = 0;
        string? copiedText = null;
        Exception? failure = null;
        using var session = new NativeCaptureSession(canvas,
            _ => throw new InvalidOperationException("OCR shortcut copied an image"),
            _ => throw new InvalidOperationException("OCR shortcut saved an image"),
            language: "zh-CN", textService: service, textMode: "ocr",
            copyText: text => { copiedText = text; copied = clock.Elapsed.TotalMilliseconds; });
        using var paint = new FirstPaint(canvas.Window, () => canvas.Window.BeginInvoke((Action)(() => {
            try {
                if (!session.Ready) throw new InvalidOperationException("Selection is not ready after first paint");
                firstPaint = clock.Elapsed.TotalMilliseconds;
                SendMessageW(canvas.Window.Handle, 0x0200, IntPtr.Zero, At(40, 40));
                SendMessageW(canvas.Window.Handle, 0x0201, new IntPtr(1), At(40, 40));
                SendMessageW(canvas.Window.Handle, 0x0200, new IntPtr(1), At(299, 104));
                SendMessageW(canvas.Window.Handle, 0x0202, IntPtr.Zero, At(299, 104));
                selectionCommitted = clock.Elapsed.TotalMilliseconds;
                if (canvas.Selection != new Rectangle(40, 40, 260, 65)) throw new InvalidOperationException("OCR selection bounds changed");
            } catch (Exception error) { failure = error; canvas.Window.Close(); }
        })));
        paint.Attach();
        using var timeout = new System.Windows.Forms.Timer { Interval = 50 };
        timeout.Tick += (_, _) => {
            if (session.Error is not null) failure = new InvalidOperationException(session.Error);
            if (clock.Elapsed.TotalSeconds > 60) failure = new TimeoutException("OCR copy session timed out");
            if (failure is not null) canvas.Window.Close();
        };
        var ended = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        canvas.Window.FormClosed += (_, _) => ended.TrySetResult();
        timeout.Start();
        // Silent OCR hides the canvas before inference. Match production's
        // FormClosed lifetime; a ShowDialog loop would end at Hide instead.
        canvas.Window.Show();
        await ended.Task;
        timeout.Stop();
        if (failure is not null) throw failure;
        if (!session.Copied || session.ResultPresented || copiedText != "Hello 123" || firstPaint <= 0 || service.Finished <= service.Started)
            throw new InvalidOperationException("OCR shortcut did not complete the expected copy handoff: " + copiedText);
        return new {
            phase, firstPaintMs = firstPaint,
            selectionInputMs = selectionCommitted - firstPaint,
            selectionRenderMs = service.Started - selectionCommitted,
            recognitionMs = service.Finished - service.Started,
            copyHandoffMs = copied - service.Finished,
            selectionToCopyMs = copied - selectionCommitted,
            sessionToCopyMs = copied,
            sessionClosedMs = clock.Elapsed.TotalMilliseconds
        };
    }
}
