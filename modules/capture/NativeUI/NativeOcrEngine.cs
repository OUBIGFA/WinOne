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
    Task<PinTranslationResult> TranslateImage(Bitmap image, CancellationToken cancellation) => Task.FromException<PinTranslationResult>(new InvalidOperationException("translationUnavailable"));
}

// Each invocation owns an offline native worker. Its models are released after
// recognition, and selected images travel only through the process pipes.
internal sealed class NativeOcrEngine(string? root = null, string? worker = null)
{
    internal async Task<string> Recognize(Bitmap image, CancellationToken cancellation)
    {
        var result = await Run(image, false, cancellation).ConfigureAwait(false);
        var text = result.GetProperty("text").GetString();
        if (string.IsNullOrWhiteSpace(text)) throw new InvalidOperationException("ocrEmpty");
        return text;
    }

    internal async Task<PinOcrLayout> PinLayout(Bitmap image, CancellationToken cancellation)
    {
        var result = await Run(image, true, cancellation).ConfigureAwait(false);
        return result.GetProperty("layout").Deserialize<PinOcrLayout>(SettingsStore.Json)
            ?? throw new InvalidDataException("ocrFailed");
    }

    private async Task<JsonElement> Run(Bitmap image, bool pinLayout, CancellationToken cancellation)
    {
        var directory = root ?? Path.Combine(AppContext.BaseDirectory, "Ocr");
        var executable = worker ?? Path.Combine(AppContext.BaseDirectory, "WinOne.Capture.exe");
        if (!File.Exists(executable) || !File.Exists(Path.Combine(directory, "rapidocrlite.dll")))
            throw new InvalidOperationException("ocrUnavailable");
        using var deadline = CancellationTokenSource.CreateLinkedTokenSource(cancellation);
        deadline.CancelAfter(TimeSpan.FromSeconds(60));
        var token = deadline.Token;
        var info = new ProcessStartInfo(executable) { UseShellExecute = false, CreateNoWindow = true,
            RedirectStandardInput = true, RedirectStandardOutput = true, RedirectStandardError = true,
            StandardOutputEncoding = Encoding.UTF8, StandardErrorEncoding = Encoding.UTF8,
            WorkingDirectory = directory };
        info.ArgumentList.Add("--ocr-worker");
        info.ArgumentList.Add(directory);
        if (pinLayout) info.ArgumentList.Add("--pin-layout");
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
            return document.RootElement.Clone();
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

    public Task<PinTranslationResult> TranslateImage(Bitmap image, CancellationToken cancellation)
    {
        if (host is null) return Task.FromException<PinTranslationResult>(new InvalidOperationException("translationUnavailable"));
        // OCR and image composition run away from the canvas thread. The caller
        // keeps the selected pixels alive until the request completes.
        return Task.Run(async () => {
            var layout = await engine.PinLayout(image, cancellation).ConfigureAwait(false);
            return await PinTranslationEngine.Translate(image, layout, null, host.PinTargetAsync, host.TranslateAsync, cancellation).ConfigureAwait(false);
        }, cancellation);
    }
}
