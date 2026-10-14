using System.Runtime.InteropServices;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

internal static class NativeLastCaptureTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    public static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        var settings = new SettingsStore(directory);
        settings.Save(settings.Snapshot() with { OutputDirectory = Path.Combine(directory,"images"), CopyToClipboard = false, AskWhereToSave = false });
        Bitmap? copied = null;
        var operations = new CaptureOperations(settings,directory,image => copied = new Bitmap(image));
        var drafts = new NativeCaptureDraftStore(directory);
        using var source = new Bitmap(240,160);
        using (var graphics = Graphics.FromImage(source)) graphics.Clear(Color.White);
        var saved = operations.CreateNativeOutput(CancellationToken.None).Save(source);
        var original = File.ReadAllBytes(saved.Path);
        try {
            NativeCaptureWorkflow.EditLast(settings,operations,new string('f',32),null,CancellationToken.None,drafts,
                show: (_,_) => throw new Exception("Stale image opened an editor")).GetAwaiter().GetResult();
            throw new Exception("Stale last-image ID was accepted");
        } catch (InvalidOperationException error) when (error.Message == "截图已变化，请刷新后重试") { }
        using var interrupted = new CancellationTokenSource();
        Bitmap? expected = null;
        void Show(WinOneCaptureCanvas canvas, Action action) {
            Exception? failure = null;
            using var timer = new System.Windows.Forms.Timer { Interval=5000 };
            timer.Tick += (_,_) => { failure = new TimeoutException("Last-image editor did not close"); canvas.Window.Close(); };
            canvas.Window.Shown += (_,_) => canvas.Window.BeginInvoke((Action)(() => {
                try { action(); } catch (Exception error) { failure=error;canvas.Window.Close(); }
            }));
            timer.Start();canvas.ShowDialog();timer.Stop();
            if (failure is not null) throw failure;
        }
        try {
            try {
                NativeCaptureWorkflow.EditLast(settings,operations,saved.Id,"en-US",interrupted.Token,drafts,
                    show: (canvas,_) => Show(canvas,() => {
                        if (canvas.Selection != new Rectangle(Point.Empty,source.Size)) throw new Exception("Last image was not fully selected");
                        canvas.AddRectangle(new Rectangle(20,20,80,50));expected=canvas.RenderSelection();interrupted.Cancel();
                    })).GetAwaiter().GetResult();
                throw new Exception("Interrupted editing returned success");
            } catch (OperationCanceledException) when (interrupted.IsCancellationRequested) { }
            var recovered = drafts.Recover() ?? throw new Exception("Last-image edits were not recoverable");
            if (recovered.Document.Layers.Length!=1) throw new Exception("Last-image annotation was lost");
            try {
                NativeCaptureWorkflow.EditLast(settings,operations,saved.Id,null,CancellationToken.None,drafts).GetAwaiter().GetResult();
                throw new Exception("Another edit replaced the pending draft");
            } catch (InvalidOperationException error) when (error.Message == "请先完成或放弃当前截图草稿") { }
            NativeCaptureWorkflow.Resume(settings,operations,recovered.Id,"en-US",CancellationToken.None,drafts,
                show: (canvas,_) => Show(canvas,() => SendMessageW(canvas.Window.Handle,0x0203,IntPtr.Zero,IntPtr.Zero))).GetAwaiter().GetResult();
            if (copied is null || expected is null || copied.Size!=expected.Size) throw new Exception("Last-image copy is missing");
            for (var y=0;y<copied.Height;y++) for (var x=0;x<copied.Width;x++)
                if (copied.GetPixel(x,y)!=expected.GetPixel(x,y)) throw new Exception("Recovered last-image pixels changed");
            if (!File.ReadAllBytes(saved.Path).SequenceEqual(original) || drafts.Recover() is not null || operations.GetDraft() is not null)
                throw new Exception("Editing overwrote the saved image or retained a completed draft");
            File.WriteAllText(Path.Combine(directory,"verification.json"),"{\"nativeEdit\":true,\"staleRejected\":true,\"recoverable\":true,\"originalPreserved\":true,\"outputPixelsIdentical\":true}");
        } finally { expected?.Dispose();copied?.Dispose(); }
    }
}
