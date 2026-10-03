internal static class CaptureCompletionTests
{
    public static void Run()
    {
        var renders = 0; var attempts = 0; var closes = 0;
        CaptureCompletion? completion = null;
        completion = new CaptureCompletion(() => { renders++; return new Bitmap(12, 8); }, image => {
            if (image.Size != new Size(12, 8)) throw new Exception("Wrong capture");
            if (++attempts == 1) throw new InvalidOperationException("Clipboard busy");
            completion!.Complete();
        }, () => closes++);
        try { completion.Complete(); throw new Exception("Failed copy was accepted"); }
        catch (InvalidOperationException error) when (error.Message == "Clipboard busy") { }
        if (completion.Committed || completion.CloseRequested || closes != 0) throw new Exception("Copy failure closed the screenshot");
        completion.Complete(); completion.Complete();
        if (attempts != 2 || renders != 2 || closes != 1 || !completion.CloseRequested) throw new Exception("Confirmation was duplicated");
        var copies = 0; var closeAttempts = 0;
        var retryClose = new CaptureCompletion(() => new Bitmap(1, 1), _ => copies++, () => { if (++closeAttempts == 1) throw new InvalidOperationException("Close failed"); });
        try { retryClose.Complete(); } catch (InvalidOperationException) { }
        retryClose.Complete();
        if (copies != 1 || closeAttempts != 2) throw new Exception("Close retry copied again");
    }
}
