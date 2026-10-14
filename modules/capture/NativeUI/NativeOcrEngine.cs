using System.Diagnostics;
using System.Drawing.Imaging;
using System.Text;
using System.Text.Json;

namespace WinOne.Capture.NativeUI;

internal interface ICaptureTextService
{
    Task<string> Recognize(Bitmap image, CancellationToken cancellation);
    Task<string> Translate(string text, CancellationToken cancellation);
    Task<string> OpenResult(Bitmap image, bool translate, CancellationToken cancellation) => Task.FromException<string>(new InvalidOperationException("resultUnavailable"));
}

// Each OCR invocation owns a short-lived offline process. No model or Python
// runtime remains resident after recognition, and no selected image hits disk.
internal sealed class NativeOcrEngine(string? root = null, string? python = null)
{
    internal async Task<string> Recognize(Bitmap image, CancellationToken cancellation)
    {
        var directory = root ?? Path.Combine(AppContext.BaseDirectory, "Ocr");
        var executable = python ?? Path.Combine(directory, "python", "python.exe");
        if (!File.Exists(executable) || !File.Exists(Path.Combine(directory, "recognize.py")))
            throw new InvalidOperationException("ocrUnavailable");
        using var deadline = CancellationTokenSource.CreateLinkedTokenSource(cancellation);
        deadline.CancelAfter(TimeSpan.FromSeconds(60));
        var token = deadline.Token;
        var info = new ProcessStartInfo(executable) { UseShellExecute = false, CreateNoWindow = true,
            RedirectStandardInput = true, RedirectStandardOutput = true, RedirectStandardError = true,
            StandardOutputEncoding = Encoding.UTF8, StandardErrorEncoding = Encoding.UTF8,
            WorkingDirectory = directory };
        info.ArgumentList.Add("-B");
        info.ArgumentList.Add(Path.Combine(directory, "recognize.py"));
        info.ArgumentList.Add(Path.Combine(directory, "models"));
        info.Environment["PYTHONUTF8"] = "1";
        using var process = new Process { StartInfo = info };
        cancellation.ThrowIfCancellationRequested();
        if (!process.Start()) throw new InvalidOperationException("ocrUnavailable");
        using var cancel = token.Register(() => { try { if (!process.HasExited) process.Kill(true); } catch (InvalidOperationException) { } });
        var output = process.StandardOutput.ReadToEndAsync();
        var error = process.StandardError.ReadToEndAsync();
        try {
            using var png = new MemoryStream();
            image.Save(png, ImageFormat.Png); png.Position = 0;
            await png.CopyToAsync(process.StandardInput.BaseStream, token);
            process.StandardInput.Close();
            await process.WaitForExitAsync(token);
            var result = await output;
            _ = await error; // Drain diagnostics without logging captured text.
            token.ThrowIfCancellationRequested();
            if (process.ExitCode != 0) throw new InvalidOperationException("ocrFailed");
            using var document = JsonDocument.Parse(result);
            var text = document.RootElement.GetProperty("text").GetString();
            if (string.IsNullOrWhiteSpace(text)) throw new InvalidOperationException("ocrEmpty");
            return text;
        } catch (OperationCanceledException) when (!cancellation.IsCancellationRequested) { throw new InvalidOperationException("ocrTimeout"); }
        finally {
            if (!process.HasExited) process.Kill(true);
            await process.WaitForExitAsync();
        }
    }
}

internal sealed class NativeCaptureTextService(NativeHostConfiguration? host) : ICaptureTextService
{
    private readonly NativeOcrEngine engine = new();
    public async Task<string> Recognize(Bitmap image, CancellationToken cancellation)
    {
        if (host is not null)
            return await host.RecognizeAsync(image, cancellation).ConfigureAwait(false);
        return await engine.Recognize(image, cancellation).ConfigureAwait(false);
    }
    public Task<string> Translate(string text, CancellationToken cancellation) => host?.TranslateAsync(text, cancellation)
        ?? Task.FromException<string>(new InvalidOperationException("translationUnavailable"));
    public Task<string> OpenResult(Bitmap image,bool translate,CancellationToken cancellation) => host?.OpenResultAsync(image,translate,cancellation)
        ?? Task.FromException<string>(new InvalidOperationException("resultUnavailable"));
}
