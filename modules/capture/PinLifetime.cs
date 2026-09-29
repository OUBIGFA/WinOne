namespace ShareX;

// Keep pins on the worker's existing STA instead of creating another application loop.
public partial class PinToScreenForm
{
    public static int ActiveCount { get { lock (syncLock) return forms.Count; } }
    public static void OpenManaged(Image image, PinToScreenOptions options)
    {
        var form = new PinToScreenForm(image, options);
        lock (syncLock) forms.Add(form);
        form.FormClosed += (_, _) => { lock (syncLock) forms.Remove(form); };
        try { form.Show(); }
        catch { lock (syncLock) forms.Remove(form); form.Dispose(); throw; }
    }
}
