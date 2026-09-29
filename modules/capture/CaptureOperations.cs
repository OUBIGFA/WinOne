using System.Drawing.Imaging;
using System.Text.Json;
using ShareX;
using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture;

internal record CaptureOutput(string Path, int Width, int Height, bool Copied, bool Pinned);

internal sealed class CaptureOperations
{
    private readonly SettingsStore store;
    private readonly string lastPath;
    private volatile CaptureOutput? last;
    public CaptureOutput? Last => last;
    public CaptureOperations(SettingsStore store, string directory)
    {
        this.store = store; lastPath = Path.Combine(directory, "last-capture.json");
        if (File.Exists(lastPath))
        {
            try { last = JsonSerializer.Deserialize<CaptureOutput>(File.ReadAllBytes(lastPath), SettingsStore.Json); }
            catch (JsonException error) { Console.Error.WriteLine("Last capture metadata: " + error.Message); }
        }
    }
    public object Capture(string mode, CancellationToken cancellation)
    {
        var settings = store.Snapshot(); ApplyTheme(settings);
        Bitmap? image;
        switch (mode)
        {
            case "region":
            case "window":
                using (var selector = new RegionCaptureForm(mode == "window" ? RegionCaptureMode.OneClick : RegionCaptureMode.Annotation, Options()))
                { selector.ShowDialog(); image = selector.GetResultImage(); }
                break;
            case "fullscreen": image = new Screenshot().CaptureFullscreen(); break;
            case "scroll":
                using (var scrolling = new ScrollingWindow(settings))
                { image = scrolling.ShowDialog() == DialogResult.OK ? scrolling.ResultCopy() : null; }
                break;
            default: throw new InvalidOperationException("不支持的截图方式");
        }
        if (image is null) throw new OperationCanceledException("截图已取消");
        using (image)
        {
            cancellation.ThrowIfCancellationRequested();
            if (settings.OpenEditor) return Edit(image, cancellation);
            return Export(image, settings, cancellation);
        }
    }
    public object EditLast(CancellationToken cancellation)
    {
        ApplyTheme(store.Snapshot());
        using var image = LoadLast(); return Edit(image, cancellation);
    }
    private object Edit(Bitmap image, CancellationToken cancellation)
    {
        using var editor = new RegionCaptureForm(RegionCaptureMode.TaskEditor, Options(), new Bitmap(image));
        editor.SaveImageRequested += (output, path) => { using (output) return SaveEditorOutput(output, path, false); };
        editor.SaveImageAsRequested += (output, path) => { using (output) return SaveEditorOutput(output, path, true); };
        editor.CopyImageRequested += Copy;
        editor.ShowDialog();
        if (editor.Result is not (RegionResult.AnnotateContinueTask or RegionResult.AnnotateRunAfterCaptureTasks)) throw new OperationCanceledException("编辑已取消");
        using var result = editor.Result == RegionResult.AnnotateContinueTask ? new Bitmap(editor.Canvas) : editor.GetResultImage();
        if (result is null) throw new InvalidOperationException("未生成编辑结果");
        return Export(result, store.Snapshot(), cancellation, editor.ImageFilePath);
    }
    private string SaveEditorOutput(Bitmap image, string? path, bool ask)
    {
        try
        {
            if (ask || string.IsNullOrEmpty(path)) path = SelectPath(store.Snapshot());
            if (string.IsNullOrEmpty(path)) return "";
            WriteImage(image, path); return path;
        }
        catch (Exception error) { MessageBox.Show(error.Message, "保存失败", MessageBoxButtons.OK, MessageBoxIcon.Error); return ""; }
    }
    internal CaptureOutput Export(Bitmap image, CaptureSettings settings, CancellationToken cancellation, string? path = null)
    {
        cancellation.ThrowIfCancellationRequested();
        if (string.IsNullOrEmpty(path)) path = settings.AskWhereToSave ? SelectPath(settings) : Path.Combine(settings.OutputDirectory, $"WinOne-{DateTime.Now:yyyyMMdd-HHmmss}-{Guid.NewGuid():N}.png");
        if (string.IsNullOrEmpty(path)) throw new OperationCanceledException("保存已取消");
        WriteImage(image, path);
        last = new(path, image.Width, image.Height, false, false);
        PersistLast();
        if (settings.CopyToClipboard)
        {
            try { Copy(image); last = last with { Copied = true }; }
            catch (Exception error) { throw new InvalidOperationException($"截图已保存到 {path}，复制失败：{error.Message}", error); }
        }
        if (settings.PinAfterCapture) { PinToScreenForm.OpenManaged(new Bitmap(image), PinOptions()); last = last with { Pinned = true }; }
        PersistLast(); return last;
    }
    public object CopyLast() { using var image = LoadLast(); Copy(image); last = last! with { Copied = true }; PersistLast(); return last; }
    public object PinLast() { ApplyTheme(store.Snapshot()); using var image = LoadLast(); PinToScreenForm.OpenManaged(new Bitmap(image), PinOptions()); return new { pins = PinToScreenForm.ActiveCount }; }
    private Bitmap LoadLast()
    {
        var output = last ?? throw new InvalidOperationException("还没有截图");
        using var source = new Bitmap(output.Path); return new Bitmap(source);
    }
    private void PersistLast() => SettingsStore.AtomicWrite(lastPath, JsonSerializer.SerializeToUtf8Bytes(last, SettingsStore.Json));
    private static RegionCaptureOptions Options() => new() { EnableSharing = false, ShowEditorPanTip = false, EditorAutoCopyImage = false };
    private static PinToScreenOptions PinOptions() => new() { Shadow = false, BorderColor = Color.FromArgb(100, 100, 100), BorderSize = 1 };
    private static void Copy(Bitmap image)
    {
        if (!ClipboardHelpers.CopyImage(image)) throw new InvalidOperationException("剪贴板暂不可用，请重试复制");
    }
    private static void WriteImage(Bitmap image, string path)
    {
        if (!Path.IsPathFullyQualified(path) || !path.EndsWith(".png", StringComparison.OrdinalIgnoreCase)) throw new InvalidOperationException("请选择 PNG 文件的完整保存路径");
        using var bytes = new MemoryStream(); image.Save(bytes, ImageFormat.Png); SettingsStore.AtomicWrite(path, bytes.ToArray());
    }
    private static string? SelectPath(CaptureSettings settings)
    {
        using var dialog = new SaveFileDialog { Title = "保存截图", Filter = "PNG 图像|*.png", DefaultExt = "png", InitialDirectory = settings.OutputDirectory, FileName = $"WinOne-{DateTime.Now:yyyyMMdd-HHmmss}.png", OverwritePrompt = true };
        return dialog.ShowDialog() == DialogResult.OK ? dialog.FileName : null;
    }
    private static void ApplyTheme(CaptureSettings settings)
    {
        var dark = settings.Theme == "dark";
        if (settings.Theme == "system")
        {
            using var key = Microsoft.Win32.Registry.CurrentUser.OpenSubKey(@"Software\Microsoft\Windows\CurrentVersion\Themes\Personalize");
            dark = key?.GetValue("AppsUseLightTheme") is int value && value == 0;
        }
        ShareXResources.Theme = dark ? ShareXTheme.DarkTheme : ShareXTheme.LightTheme;
    }
}
