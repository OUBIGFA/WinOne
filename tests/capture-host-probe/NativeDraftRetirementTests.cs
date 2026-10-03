using System.Text.Json;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;

internal static class NativeDraftRetirementTests
{
    public static int Run(string output)
    {
        if (Directory.Exists(output)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(output); output = Path.GetFullPath(output);
        using var image = new Bitmap(100, 80);
        using var canvas = new WinOneCaptureCanvas(image);
        canvas.SetRegion(new Rectangle(0, 0, 100, 80));
        var document = canvas.ExportDocument();
        var settings = new CaptureSettings { OutputDirectory = Path.Combine(output, "exports") };
        VerifyEncodingCancellation(Path.Combine(output, "encoding-race"), image, settings, document);
        VerifyInterruptedCommit(Path.Combine(output, "interrupted-commit"), image, settings, document);
        var store = new NativeCaptureDraftStore(output);
        var metadata = Path.Combine(output, "native-capture", "current.json");
        var ledger = Path.Combine(output, "native-capture", "retired.json");
        string ImagePath(NativeCaptureDraft draft) => Path.Combine(output, "native-capture", "images", draft.Id + ".png");

        var first = store.Begin(image, settings, document, "region");
        var attributes = File.GetAttributes(metadata);
        File.SetAttributes(metadata, attributes | FileAttributes.ReadOnly);
        try {
            var pending = store.Retire(first.Id, first.Revision);
            if (!pending.Pending || pending.Errors.Count == 0 || !File.Exists(metadata) || !File.Exists(ImagePath(first))) throw new Exception("Blocked metadata cleanup was not retained");
            if (new NativeCaptureDraftStore(output).Read() is not null) throw new Exception("Retired draft reappeared after reopening storage");
        } finally { File.SetAttributes(metadata, attributes); }
        var second = store.Begin(image, settings, document, "region");
        var cleaned = store.RetryCleanup();
        if (cleaned.Pending || File.Exists(ImagePath(first)) || !File.Exists(ImagePath(second)) || store.Read()?.Id != second.Id) throw new Exception("Old cleanup removed or replaced the new draft");
        try { store.Update(first.Id, first.Revision, document); throw new Exception("Late old update was accepted"); } catch (InvalidOperationException) { }
        try { store.Retire(first.Id, first.Revision); throw new Exception("Late old retirement was accepted"); } catch (InvalidOperationException) { }

        var source = ImagePath(second); var imageAttributes = File.GetAttributes(source);
        File.SetAttributes(source, imageAttributes | FileAttributes.ReadOnly);
        try {
            var pending = store.Retire(second.Id, second.Revision);
            if (!pending.Pending || store.Read() is not null || !File.Exists(ledger) || !File.Exists(source)) throw new Exception("Blocked image cleanup lost its durable retirement");
        } finally { File.SetAttributes(source, imageAttributes); }
        if (new NativeCaptureDraftStore(output).RetryCleanup().Pending || File.Exists(source) || File.Exists(ledger)) throw new Exception("Retired image could not be cleaned after release");

        var third = store.Begin(image, settings, document, "region");
        File.WriteAllText(ledger, "[]"); var ledgerAttributes = File.GetAttributes(ledger);
        File.SetAttributes(ledger, ledgerAttributes | FileAttributes.ReadOnly);
        try {
            try { store.Retire(third.Id, third.Revision); throw new Exception("Failed retirement marker was reported as successful"); }
            catch (UnauthorizedAccessException) { } catch (IOException) { }
            if (store.Read()?.Id != third.Id || !File.Exists(ImagePath(third))) throw new Exception("Failed retirement damaged the active draft");
        } finally { File.SetAttributes(ledger, ledgerAttributes); }
        if (store.Retire(third.Id, third.Revision).Pending) throw new Exception("Retirement retry failed");

        var missing = store.Begin(image, settings, document, "region");
        File.Move(ImagePath(missing), Path.Combine(output, "missing-original.png"));
        if (store.Retire(missing.Id, missing.Revision).Pending || store.Read() is not null) throw new Exception("Missing original prevented explicit retirement");
        var outside = Path.Combine(output, "unrelated.txt"); File.WriteAllText(outside, "keep");
        File.WriteAllText(ledger, "[{\"id\":\"../unrelated\",\"revision\":0}]");
        if (!store.RetryCleanup().Pending || File.ReadAllText(outside) != "keep") throw new Exception("Invalid cleanup identity affected unrelated data");
        try { store.Begin(image, settings, document, "region"); throw new Exception("Corrupt cleanup journal was ignored during creation"); }
        catch (InvalidDataException) { }
        File.Delete(ledger);
        File.WriteAllText(Path.Combine(output, "report.json"), JsonSerializer.Serialize(new { metadataFailureRetained = true, noResurrection = true, newDraftProtected = true, imageCleanupRetry = true, failedMarkerPreserved = true, missingOriginalRetired = true }));
        Console.WriteLine("PASS: native draft retirement, durable cleanup retry, no resurrection and newer-draft protection");
        return 0;
    }

    private static void VerifyEncodingCancellation(string directory, Bitmap image, CaptureSettings settings, WinOneCaptureDocument document)
    {
        using var entered = new ManualResetEventSlim();
        using var release = new ManualResetEventSlim();
        var writes = 0;
        var store = new NativeCaptureDraftStore(directory, (pixels, path) => {
            Interlocked.Increment(ref writes);
            entered.Set();
            if (!release.Wait(TimeSpan.FromSeconds(15))) throw new TimeoutException("Encoder was not released");
            PngFiles.Write(pixels, path);
        });
        var before = store.Reserve(image.Size, settings, document, "region");
        store.CancelReservation(before.Id);
        try { store.CompleteReservation(before.Id, image); throw new Exception("Cancelled reservation reached encoding"); }
        catch (OperationCanceledException) { }
        if (writes != 0) throw new Exception("Cancelled reservation encoded pixels");

        // Hold the old encoder at a deterministic boundary while a new capture
        // commits. No sleeps, live desktop or shared bitmap access are involved.
        var old = store.Reserve(image.Size, settings, document, "region");
        using var oldImage = new Bitmap(image);
        var encoding = Task.Run(() => store.CompleteReservation(old.Id, oldImage));
        NativeCaptureDraft? next = null;
        try {
            if (!entered.Wait(TimeSpan.FromSeconds(10))) throw new TimeoutException("Encoder did not start");
            try { new NativeCaptureDraftStore(directory).Recover(); throw new Exception("Recovery touched an active encoder"); }
            catch (IOException error) when ((error.HResult & 0xffff) == 32) { }
            if (!store.CancelReservation(old.Id).Pending || store.ReadReservation() is not null || store.Read() is not null)
                throw new Exception("Cancellation did not immediately hide the in-flight draft");
            next = new NativeCaptureDraftStore(directory).Begin(image, settings, document, "region");
        } finally {
            release.Set();
            try { encoding.GetAwaiter().GetResult(); throw new Exception("Cancelled encoder committed"); }
            catch (OperationCanceledException) { }
        }
        var reopened = new NativeCaptureDraftStore(directory);
        if (reopened.Read()?.Id != next!.Id || reopened.ReadReservation() is not null || reopened.RetryCleanup().Pending)
            throw new Exception("Old encoding completion damaged the newer capture");
        using var restored = reopened.LoadImage(next.Id, next.Revision);
        if (restored.Size != image.Size || File.Exists(Path.Combine(directory, "native-capture", "images", old.Id + ".png")))
            throw new Exception("Cancelled pixels survived or newer pixels were lost");
        reopened.Retire(next.Id, next.Revision);
    }

    private static void VerifyInterruptedCommit(string directory, Bitmap image, CaptureSettings settings, WinOneCaptureDocument document)
    {
        var store = new NativeCaptureDraftStore(directory);
        var pending = store.Reserve(image.Size, settings, document, "region");
        try { store.Recover(); throw new Exception("Incomplete original was silently accepted"); }
        catch (NativeDraftIncompleteException error) when (error.DraftId == pending.Id) { }
        if (store.ReadReservation()?.Id != pending.Id) throw new Exception("Failed recovery discarded its pending marker");
        var originalPath = Path.Combine(Path.GetFullPath(directory), "native-capture", "images", pending.Id + ".png");
        PngFiles.Write(image, originalPath);
        var recovered = new NativeCaptureDraftStore(directory).Recover() ?? throw new Exception("Completed original was not recovered");
        if (recovered.Id != pending.Id || recovered.Revision != 0 || store.ReadReservation() is not null || store.Read()?.Id != pending.Id)
            throw new Exception("Interrupted commit changed identity or retained pending state");
        using var original = store.LoadImage(recovered.Id, recovered.Revision);
        if (original.Size != image.Size) throw new Exception("Recovery changed original dimensions");
        store.Retire(recovered.Id, recovered.Revision);
    }
}
