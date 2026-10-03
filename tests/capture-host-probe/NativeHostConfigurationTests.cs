using System.Diagnostics;
using System.IO.Pipes;
using System.Text.Json;
using System.Text.Json.Nodes;
using WinOne.Capture;

internal static class NativeHostConfigurationTests
{
    public static void Run() => RunAsync().GetAwaiter().GetResult();

    private static async Task RunAsync()
    {
        foreach (var scenario in new[] { "saved", "reset", "conflict", "wrong-job", "wrong-response", "failed", "missing-tools" }) {
            using var stop = new CancellationTokenSource(TimeSpan.FromSeconds(8));
            var pipeName = "WinOneNativeConfigTest-" + Guid.NewGuid().ToString("N");
            var writes = 0; var polls = 0; string? saveId = null; JsonObject? saved = null;
            var entered = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
            var release = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
            var serving = Task.Run(async () => {
                try {
                    while (!stop.IsCancellationRequested) {
                        using var server = new NamedPipeServerStream(pipeName, PipeDirection.InOut, 1, PipeTransmissionMode.Byte, PipeOptions.Asynchronous | PipeOptions.CurrentUserOnly);
                        await server.WaitForConnectionAsync(stop.Token);
                        using var request = JsonDocument.Parse(await PipeProtocol.ReadAsync(server, stop.Token));
                        var root = request.RootElement;
                        var id = root.GetProperty("id").GetString();
                        object response;
                        switch (root.GetProperty("method").GetString()) {
                            case "capture.settings.get":
                                response = new { id, result = new { schemaVersion = 1, revision = 42, outputDirectory = "X:\\Pictures", copyToClipboard = false, fileNameTemplate = "work-{date}", toolbarTools = new[] { "arrow" }, futurePreference = "preserved" } };
                                break;
                            case "capture.settings.save":
                                writes++; saveId = id; saved = JsonNode.Parse(root.GetProperty("params").GetRawText())!.AsObject();
                                if (scenario == "saved") { entered.SetResult(); await release.Task.WaitAsync(stop.Token); }
                                if (scenario == "conflict") response = new { id, error = new { code = "configuration", message = "revision conflict" } };
                                else response = new { id, result = new { configurationJob = "job-1" } };
                                break;
                            case "configuration.jobs.get":
                                polls++;
                                if (polls == 1 && scenario is "saved" or "reset") response = new { id, result = new { id = "job-1", state = "running" } };
                                else {
                                    var confirmed = saved!.DeepClone().AsObject(); confirmed["revision"] = 43;
                                    if (scenario == "missing-tools") confirmed.Remove("toolbarTools");
                                    object terminal = scenario == "failed" ? new { id = saveId, error = new { code = "configuration", message = "apply failed" } }
                                        : new { id = scenario == "wrong-response" ? "another-save" : saveId, result = confirmed };
                                    response = new { id, result = new { id = scenario == "wrong-job" ? "another-job" : "job-1", state = scenario == "failed" ? "failed" : "completed", response = terminal } };
                                }
                                break;
                            default: throw new Exception("Unexpected configuration request");
                        }
                        await PipeProtocol.WriteAsync(server, JsonSerializer.SerializeToUtf8Bytes(response), stop.Token);
                    }
                } catch (OperationCanceledException) when (stop.IsCancellationRequested) { }
            });
            try {
                using var parent = Process.GetCurrentProcess();
                var client = new NativeHostConfiguration(pipeName, parent);
                var idleEvents = 0; client.BecameIdle += () => Interlocked.Increment(ref idleEvents);
                var desired = scenario == "reset" ? null : Array.Empty<string>();
                try {
                    var pending = client.SaveToolbarAsync(desired, stop.Token);
                    if (scenario == "saved") {
                        await entered.Task.WaitAsync(stop.Token);
                        if (!client.IsSaving || client.TryBeginShutdown()) throw new Exception("Active save did not hold the worker alive");
                        try { await client.SaveToolbarAsync(desired, stop.Token); throw new Exception("Concurrent toolbar save was admitted"); }
                        catch (InvalidOperationException) { }
                        release.SetResult();
                    }
                    var result = await pending;
                    if (scenario is not ("saved" or "reset")) throw new Exception("Invalid configuration result was accepted: " + scenario);
                    if ((result is null) != (desired is null) || (result is not null && result.Length != 0)) throw new Exception("Null and empty toolbar preferences were conflated");
                } catch (Exception error) when (scenario is not ("saved" or "reset") && error is InvalidDataException or InvalidOperationException) { }
                if (writes != 1 || saved is null) throw new Exception("Configuration write was missing or retried");
                if (saved["revision"]!.GetValue<long>() != 42 || saved["futurePreference"]!.GetValue<string>() != "preserved" || saved["copyToClipboard"]!.GetValue<bool>())
                    throw new Exception("Toolbar save overwrote unrelated settings or revision");
                if (scenario == "conflict" && polls != 0) throw new Exception("Rejected save was polled or retried");
                if (client.IsSaving || idleEvents != 1 || !client.TryBeginShutdown()) throw new Exception("Configuration lifetime did not release after completion");
                try { await client.SaveToolbarAsync(desired, stop.Token); throw new Exception("Save admitted after shutdown"); }
                catch (InvalidOperationException) { }
            } finally { stop.Cancel(); await serving; }
        }
        // Use an actual pipe to verify that the server PID, not JSON metadata,
        // controls the trust check. No foreign process needs to be started.
        var name = "WinOneNativeIdentityTest-" + Guid.NewGuid().ToString("N");
        using var real = new NamedPipeServerStream(name, PipeDirection.InOut, 1, PipeTransmissionMode.Byte, PipeOptions.Asynchronous);
        var accepting = real.WaitForConnectionAsync();
        using var connection = new NamedPipeClientStream(".", name, PipeDirection.InOut, PipeOptions.Asynchronous);
        await connection.ConnectAsync(3000); await accepting;
        try { NativeHostConfiguration.VerifyServer(connection, Environment.ProcessId + 1); throw new Exception("Wrong Host identity was accepted"); }
        catch (InvalidOperationException) { }
        NativeHostConfiguration.VerifyServer(connection, Environment.ProcessId);
    }
}
