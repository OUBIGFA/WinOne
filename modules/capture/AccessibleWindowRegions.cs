using System.Diagnostics;
using System.Globalization;

namespace WinOne.Capture;

internal static class AccessibleWindowRegions
{
    private sealed class ProviderException(int code) : IOException($"Region helper failed ({code})") { internal int Code { get; } = code; }
    internal static Rectangle Parse(string line, Rectangle clip)
    {
        var parts = line.Split(' ', StringSplitOptions.RemoveEmptyEntries);
        if (parts.Length != 4) throw new InvalidDataException("Invalid region response");
        var values = parts.Select(value => int.Parse(value, NumberStyles.AllowLeadingSign, CultureInfo.InvariantCulture)).ToArray();
        var width = (long)values[2] - values[0]; var height = (long)values[3] - values[1];
        if (width <= 0 || height <= 0 || width > int.MaxValue || height > int.MaxValue) throw new InvalidDataException("Invalid region bounds");
        return Rectangle.Intersect(new Rectangle(values[0], values[1], (int)width, (int)height), clip);
    }

    internal static Task<IReadOnlyList<Rectangle>> ReadAsync(WindowCaptureTarget target, CancellationToken cancellation, string? helper = null, TimeSpan? timeout = null)
        => Task.Run(async () => {
            using var budget = CancellationTokenSource.CreateLinkedTokenSource(cancellation);
            budget.CancelAfter(timeout ?? TimeSpan.FromSeconds(4));
            try
            {
                try { return await ReadCoreAsync(target, budget.Token, helper, timeout).ConfigureAwait(false); }
                catch (ProviderException error) when (error.Code == 5)
                {
                    // Providers can still be initializing after the first UIA
                    // connection. One retry shares the original time budget.
                    Console.Error.WriteLine("Window region provider was unavailable; retrying once");
                    await Task.Delay(100, budget.Token).ConfigureAwait(false);
                    return await ReadCoreAsync(target, budget.Token, helper, timeout).ConfigureAwait(false);
                }
            }
            catch (OperationCanceledException) when (!cancellation.IsCancellationRequested) { throw new TimeoutException("Window region detection timed out"); }
        }, cancellation);

    private static async Task<IReadOnlyList<Rectangle>> ReadCoreAsync(WindowCaptureTarget target, CancellationToken cancellation, string? helper, TimeSpan? timeout)
    {
        cancellation.ThrowIfCancellationRequested();
        using var deadline = CancellationTokenSource.CreateLinkedTokenSource(cancellation);
        deadline.CancelAfter(timeout ?? TimeSpan.FromSeconds(4));
        var start = new ProcessStartInfo(helper ?? Path.Combine(AppContext.BaseDirectory, "WinOne.Capture.Regions.exe")) {
            UseShellExecute = false, CreateNoWindow = true, RedirectStandardOutput = true
        };
        start.ArgumentList.Add(target.Handle.ToInt64().ToString(CultureInfo.InvariantCulture));
        start.ArgumentList.Add(Environment.ProcessId.ToString(CultureInfo.InvariantCulture));
        using var process = Process.Start(start) ?? throw new IOException("Region helper did not start");
        try
        {
            var result = new HashSet<Rectangle>(); var count = 0;
            while (await process.StandardOutput.ReadLineAsync(deadline.Token).ConfigureAwait(false) is { } line)
            {
                if (++count > 8192 || line.Length > 128) throw new InvalidDataException("Region response exceeded limits");
                var rectangle = Parse(line, target.Bounds);
                if (rectangle.Width >= 12 && rectangle.Height >= 12) result.Add(rectangle);
            }
            await process.WaitForExitAsync(deadline.Token).ConfigureAwait(false);
            if (process.ExitCode != 0) throw new ProviderException(process.ExitCode);
            return result.OrderBy(rectangle => (long)rectangle.Width * rectangle.Height).ToArray();
        }
        catch (OperationCanceledException) when (!cancellation.IsCancellationRequested)
        {
            throw new TimeoutException("Window region detection timed out");
        }
        finally
        {
            // Only this request's helper is terminated. Closing a selector must
            // not leave a provider query or a helper process running behind it.
            if (!process.HasExited)
            {
                try { process.Kill(entireProcessTree: true); }
                catch (InvalidOperationException) when (process.HasExited) { }
                await process.WaitForExitAsync().WaitAsync(TimeSpan.FromSeconds(5)).ConfigureAwait(false);
            }
        }
    }
}
