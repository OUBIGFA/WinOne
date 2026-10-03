namespace WinOne.Capture.NativeUI;

internal static class NativeCaptureRecovery
{
    public static NativeCaptureDraft? Read(NativeCaptureDraftStore store, string? language,
        CancellationToken cancellation, Func<bool>? chooseRestart = null)
    {
        cancellation.ThrowIfCancellationRequested();
        try { return store.Recover(); }
        catch (NativeDraftIncompleteException error) {
            if (!(chooseRestart ?? (() => ConfirmRestart(language)))()) throw new OperationCanceledException();
            cancellation.ThrowIfCancellationRequested();
            // The user's choice applies to this exact incomplete generation.
            // A changed generation fails validation instead of discarding it.
            var cleanup = store.CancelReservation(error.DraftId);
            foreach (var detail in cleanup.Errors) Console.Error.WriteLine("Native draft cleanup: " + detail);
            return null;
        }
    }

    internal static bool ConfirmRestart(string? language, Action<TaskDialogPage>? prepare = null)
    {
        var english = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase);
        var restart = new TaskDialogButton(english ? "Take a new screenshot" : "重新截图");
        var later = new TaskDialogButton(english ? "Not now" : "稍后处理");
        var page = new TaskDialogPage {
            Caption = "WinOne",
            Heading = english ? "The previous screenshot could not be saved completely" : "上次截图未保存完整",
            Text = english ? "Taking a new screenshot will discard the unfinished capture." : "重新截图将放弃这张未完成的截图。",
            AllowCancel = true,
            DefaultButton = later,
            Buttons = { restart, later }
        };
        // Native task-dialog buttons retain Windows keyboard and accessibility
        // behavior. The optional observer is used only on a private test desktop.
        prepare?.Invoke(page);
        return TaskDialog.ShowDialog(page) == restart;
    }
}
