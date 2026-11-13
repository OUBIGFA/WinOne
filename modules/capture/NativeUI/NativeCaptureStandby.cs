using ShareX.HelpersLib;

namespace WinOne.Capture.NativeUI;

// Warm the real selection/session/paint path using generated pixels only. A
// 1x1 editor warmed different code and left the first region capture cold.
internal static class NativeCaptureStandby
{
    private static volatile bool prepared;
    private static int preparations;
    internal static bool Prepared => prepared;
    internal static int Preparations => Volatile.Read(ref preparations);

    internal static void Prepare(CaptureOperations operations, CaptureSettings settings, string? language = null, bool refresh = false)
    {
        if (prepared && !refresh) return;
        CaptureOperations.ApplyTheme(settings);
        NativeCaptureWorkflow.Prepare(operations, settings, language);
        prepared = true;
        Interlocked.Increment(ref preparations);
    }
}
