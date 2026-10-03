using WinOne.Capture;
using ShareX;

internal static class NativeDraftActionTests
{
    public static void RunOnSta(string directory)
    {
        Exception? failure = null;
        var thread = new Thread(() =>
        {
            try
            {
                Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
                Application.EnableVisualStyles();
                var store = new SettingsStore(directory);
                var operations = new CaptureOperations(store, directory);
                using var image = new Bitmap(64, 48);
                using (var graphics = Graphics.FromImage(image)) graphics.Clear(Color.Coral);
                var draft = operations.PrepareEditor(image, new CaptureSettings { OutputDirectory = directory, CopyToClipboard = false });
                draft = operations.UpdateDraft(draft.Id, 0, draft.Document with { Version = 2, Crop = new AnnotationCrop(2, 2, 32, 24), Marks = [new AnnotationMark { Kind = "cover", X = 2, Y = 2, Width = 10, Height = 10, Color = "#000000" }] });
                operations.PinDraft(draft.Id, draft.Revision);
                Application.DoEvents();
                var pin = Application.OpenForms.OfType<PinToScreenForm>().Single();
                using var pixels = new Bitmap(pin.Image);
                if (pixels.Size != new Size(32, 24) || pixels.GetPixel(2, 2).ToArgb() != Color.Black.ToArgb()) throw new Exception("pinned draft did not contain edited pixels");
                if (operations.GetDraft()?.Revision != draft.Revision || operations.Last is not null) throw new Exception("pinning changed the saved capture or discarded the draft");
                PinToScreenForm.CloseAll();
                if (PinToScreenForm.ActiveCount != 0) throw new Exception("draft pin did not close");
                operations.CancelDraft(draft.Id, draft.Revision);
            }
            catch (Exception error) { failure = error; }
            finally { PinToScreenForm.CloseAll(); }
        });
        thread.SetApartmentState(ApartmentState.STA); thread.Start();
        if (!thread.Join(10000)) throw new Exception("draft action test timed out");
        if (failure is not null) throw new Exception("native draft action failed", failure);
    }
}
