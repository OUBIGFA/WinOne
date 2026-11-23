namespace ShareX;

// Keep pins on the worker's existing STA instead of creating another application loop.
public partial class PinToScreenForm
{
    public static int ActiveCount { get { lock (syncLock) return forms.Count; } }
    public static void OpenManaged(Image image, PinToScreenOptions options, string? language = null, string? fileNameTemplate = null)
        => OpenManaged(image, options, language, fileNameTemplate, null, null);

    internal static void OpenManaged(Image image, PinToScreenOptions options, string? language, string? fileNameTemplate,
        Func<bool>? translationEnabled, WinOne.Capture.NativeUI.NativeHostConfiguration? host)
    {
        PinToScreenForm? form = null;
        try
        {
            form = new PinToScreenForm(image, options);
            form.managedFileNameTemplate = fileNameTemplate;
            form.ConfigureWinOneMenu(language);
            if (translationEnabled is not null) form.SetUpTranslation(translationEnabled, host);
            lock (syncLock) forms.Add(form);
            form.FormClosed += (_, _) => { lock (syncLock) forms.Remove(form); };
            form.Show();
        }
        catch
        {
            if (form is null) image.Dispose();
            else { lock (syncLock) forms.Remove(form); form.Dispose(); }
            throw;
        }
    }
}
