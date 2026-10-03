using System.IO.Pipes;
using System.Text;
using System.Text.Json;

// Test observation is read-only IPC. Atomic file replacement can fail on
// Windows while Node holds a read handle, so live UI never rewrites a polled file.
internal sealed class NativeSnapshotServer : IDisposable
{
    private readonly CancellationTokenSource stopping = new();
    private readonly NamedPipeServerStream pipe;
    private readonly Task serving;
    private readonly string output;
    private string state;

    public NativeSnapshotServer(string output, string initial)
    {
        this.output = output; state = initial;
        var name = "WinOneNativeState-" + Guid.NewGuid().ToString("N");
        pipe = new NamedPipeServerStream(name, PipeDirection.InOut, 1, PipeTransmissionMode.Byte, PipeOptions.Asynchronous | PipeOptions.CurrentUserOnly);
        WinOne.Capture.SettingsStore.AtomicWrite(Path.Combine(output, "state.json"), Encoding.UTF8.GetBytes(initial));
        serving = Task.Run(Serve);
        WinOne.Capture.SettingsStore.AtomicWrite(Path.Combine(output, "state-pipe.json"), JsonSerializer.SerializeToUtf8Bytes(new { version = 1, name }));
    }
    public void Update(string next) => Volatile.Write(ref state, next);
    private async Task Serve()
    {
        try {
            while (!stopping.IsCancellationRequested) {
                await pipe.WaitForConnectionAsync(stopping.Token);
                try {
                    var bytes = Encoding.UTF8.GetBytes(Volatile.Read(ref state));
                    await pipe.WriteAsync(BitConverter.GetBytes(bytes.Length), stopping.Token);
                    await pipe.WriteAsync(bytes, stopping.Token);
                    // Keep the instance connected until the reader acknowledges
                    // the complete frame; Disconnect otherwise discards unread data.
                    var ack = new byte[1];
                    await pipe.ReadExactlyAsync(ack, stopping.Token);
                } catch (IOException) { /* A cancelled test reader may disconnect. */ }
                finally { if (pipe.IsConnected) pipe.Disconnect(); }
            }
        } catch (OperationCanceledException) when (stopping.IsCancellationRequested) { }
    }
    public void Dispose()
    {
        WinOne.Capture.SettingsStore.AtomicWrite(Path.Combine(output, "state.json"), Encoding.UTF8.GetBytes(Volatile.Read(ref state)));
        WinOne.Capture.SettingsStore.AtomicWrite(Path.Combine(output, "state-complete.json"), Encoding.UTF8.GetBytes("{}"));
        stopping.Cancel();
        try { serving.GetAwaiter().GetResult(); }
        finally { pipe.Dispose(); stopping.Dispose(); }
    }
}
