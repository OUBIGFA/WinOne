using ShareX.ScreenCaptureLib;

// Explicit comparison entry only. The caller supplies synthetic pixels and
// starts the process on a private desktop; this never acquires screen pixels.
internal static class MatureCaptureReference
{
    public static void Run(string imagePath)
    {
        Exception? failure = null;
        var thread = new Thread(() => {
            try {
                Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
                Application.SetUnhandledExceptionMode(UnhandledExceptionMode.ThrowException);
                Application.EnableVisualStyles();
                using var image = new Bitmap(imagePath);
                var options = new RegionCaptureOptions {
                    DetectWindows = false, DetectControls = false,
                    EnableSharing = false, QuickCrop = false,
                    EditorAutoCopyImage = false,
                };
                using var window = new RegionCaptureForm(RegionCaptureMode.Annotation, options, new Bitmap(image));
                // ShareX owns this form with ShowDialog, disposing it only
                // after the modal loop has unwound (including pending paint).
                window.ShowDialog();
            } catch (Exception error) { failure = error; File.WriteAllText(Path.Combine(Path.GetDirectoryName(imagePath)!, "reference-error.txt"), error.ToString()); }
        });
        thread.SetApartmentState(ApartmentState.STA);thread.Start();thread.Join();
        if (failure is not null) throw failure;
    }
}
