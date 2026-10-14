using ShareX.ScreenCaptureLib;
using WinOne.Capture;

internal static class NativeAutosaveTests
{
    public static int Run(string directory)
    {
        if (Directory.Exists(directory)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(directory);
        using var image = new Bitmap(100, 80);
        using var canvas = new WinOneCaptureCanvas(image);
        canvas.SetRegion(new Rectangle(0, 0, 100, 80));
        var initial = canvas.ExportDocument();
        using (var selecting = new WinOneCaptureCanvas(image)) {
            selecting.BeginRegion(new Rectangle(10, 10, 30, 30));
            if (selecting.SnapshotDocument() is not null || !selecting.RegionGestureActive)
                throw new Exception("Autosave snapshot committed an active pointer gesture");
            selecting.FinishGesture();
            if (selecting.SnapshotDocument() is null) throw new Exception("Completed gesture could not be saved");
        }
        var settings = new CaptureSettings { OutputDirectory = Path.GetFullPath(Path.Combine(directory, "exports")) };
        using var entered = new ManualResetEventSlim();
        using var release = new ManualResetEventSlim();
        var encodes = 0;
        var store = new NativeCaptureDraftStore(Path.Combine(directory, "preserved"), (pixels, path) => {
            Interlocked.Increment(ref encodes); entered.Set();
            if (!release.Wait(TimeSpan.FromSeconds(10))) throw new TimeoutException("Encoder was not released");
            PngFiles.Write(pixels, path);
        });
        var autosave = new NativeCaptureAutosave(store, new Bitmap(image), settings, initial, "region");
        var pendingText = new NativeTextInputState(null, "未确认的文字\n第二行", 0, 0, 50, 40, 80, 24, Color.Red.ToArgb(), 2, 3);
        if (!entered.Wait(TimeSpan.FromSeconds(10))) throw new TimeoutException("Encoder did not start");
        try {
            // Hundreds of edits during one blocked PNG encode must retain only
            // the last document. No user desktop or clipboard is involved.
            for (var i = 1; i <= 300; i++) autosave.Publish(initial with { Selection = new Rectangle(i % 50, 0, 50, 80) });
            autosave.Publish(new NativeCaptureSnapshot(initial with { Selection = new Rectangle(0, 0, 50, 80) }, pendingText));
        } finally { release.Set(); }
        var flushed=autosave.FlushAsync().GetAwaiter().GetResult();
        if(flushed.Text!=pendingText||flushed.Document.Selection!=new Rectangle(0,0,50,80))throw new Exception("Flush did not persist the latest queued snapshot");
        autosave.Publish(new NativeCaptureSnapshot(flushed.Document,pendingText));
        if(autosave.FlushAsync().GetAwaiter().GetResult().Revision<=flushed.Revision)throw new Exception("Flushing closed the autosave writer");
        autosave.FinishAsync(false).GetAwaiter().GetResult();
        var restored = store.Read() ?? throw new Exception("Preserved autosave disappeared");
        if (encodes != 1 || restored.Document.Selection != new Rectangle(0, 0, 50, 80)) throw new Exception("Newest edit or immutable original was lost");
        if (restored.Text != pendingText || restored.Document.Layers.Length != 0) throw new Exception("Pending text was lost or committed into drawing history");
        using (var recoveredCanvas = new WinOneCaptureCanvas(image)) {
            recoveredCanvas.RestoreDocument(restored.Document);
            using var session = new NativeCaptureSession(recoveredCanvas, _ => throw new Exception("Recovery copied an image"), _ => throw new Exception("Recovery saved an image"));
            session.RestoreText(restored.Text!);
            var input = session.TextInput ?? throw new Exception("Recovered text did not reopen inline");
            var liveStore = new NativeCaptureDraftStore(Path.Combine(directory, "live-session"));
            var liveSave = new NativeCaptureAutosave(liveStore, new Bitmap(image), settings, restored.Document, "region");
            session.SnapshotReady += liveSave.Publish;
            var handle = input.Handle;
            var snapshot = session.Snapshot() ?? throw new Exception("Inline session could not be snapshotted");
            if (snapshot.Text?.Value != pendingText.Value || snapshot.Text.SelectionStart != 2 || snapshot.Text.SelectionLength != 3 ||
                snapshot.Text.FontPixels != 24 || snapshot.Text.ColorArgb != Color.Red.ToArgb() || recoveredCanvas.AnnotationCount != 0 || input.Handle != handle)
                throw new Exception("Snapshot committed or changed the live inline editor");
            input.Text = "继续输入";
            if (session.Snapshot()?.Text?.Value != "继续输入") throw new Exception("Restored input was not editable");
            Application.DoEvents();
            session.SnapshotReady -= liveSave.Publish;
            liveSave.FinishAsync(false).GetAwaiter().GetResult();
            if (new NativeCaptureDraftStore(Path.Combine(directory, "live-session")).Read()?.Text?.Value != "继续输入")
                throw new Exception("Native input events did not reach background persistence");
        }
        try { store.Update(restored.Id, restored.Revision, restored.Document, pendingText with { SelectionLength = 4096 }); throw new Exception("Invalid pending selection was accepted"); }
        catch (InvalidDataException) { }
        if (store.Read()?.Text != pendingText) throw new Exception("Invalid input damaged the last saved text");
        using (var pixels = store.LoadImage(restored.Id, restored.Revision))
            if (pixels.Size != image.Size) throw new Exception("Autosave changed source dimensions");
        try { autosave.Publish(initial); throw new Exception("Closed autosave accepted an edit"); } catch (InvalidOperationException) { }

        entered.Reset(); release.Reset();
        var cancelStore = new NativeCaptureDraftStore(Path.Combine(directory, "cancelled"), (pixels, path) => {
            entered.Set();
            if (!release.Wait(TimeSpan.FromSeconds(10))) throw new TimeoutException("Cancelled encoder was not released");
            PngFiles.Write(pixels, path);
        });
        var cancelled = new NativeCaptureAutosave(cancelStore, new Bitmap(image), settings, initial, "region");
        if (!entered.Wait(TimeSpan.FromSeconds(10))) throw new TimeoutException("Cancel encoder did not start");
        var finishing = cancelled.FinishAsync(true);
        try {
            if (!SpinWait.SpinUntil(() => cancelStore.ReadReservation() is null, TimeSpan.FromSeconds(5))) throw new Exception("Cancel waited for PNG encoding");
            if (finishing.IsCompleted) throw new Exception("Worker released lifetime before encoder finished");
        } finally { release.Set(); }
        if (finishing.GetAwaiter().GetResult().Pending || cancelStore.Read() is not null) throw new Exception("Cancelled autosave returned a recoverable draft");

        var failureStore = new NativeCaptureDraftStore(Path.Combine(directory, "failed"), (_, _) => throw new IOException("synthetic encoder failure"));
        var failed = new NativeCaptureAutosave(failureStore, new Bitmap(image), settings, initial, "region");
        try { failed.FinishAsync(false).GetAwaiter().GetResult(); throw new Exception("Failed autosave reported success"); }
        catch (IOException error) when (error.Message == "synthetic encoder failure") { }
        if (failed.FinishAsync(true).GetAwaiter().GetResult().Pending || failureStore.ReadReservation() is not null)
            throw new Exception("Failed encoding could not be explicitly discarded");
        Console.WriteLine("PASS: background autosave coalesces edits, preserves latest state, cancels during encoding and reports failure");
        return 0;
    }
}
