using System.IO.Pipes;
using System.Text.Json;
using WinOne.Capture;

// Exercises the real worker pipe and job admission without acquiring the
// desktop or using the clipboard. Native canvas behavior is tested separately.
internal static class NativeCaptureRoutingTests
{
    public static async Task RunAsync(string directory)
    {
        Directory.CreateDirectory(directory);
        var pipeName = "WinOne-native-route-" + Guid.NewGuid().ToString("N");
        var draftId = new string('a', 32);
        using var deadline = new CancellationTokenSource(TimeSpan.FromSeconds(15));
        var ready = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        var scrollEntered = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        var scrollResult = new TaskCompletionSource<object>(TaskCreationOptions.RunContinuationsAsynchronously);
        var scrollRuns = 0; var resumeRuns = 0; var editRuns = 0;
        var lastId=new string('c',32);
        var imagePath=Path.Combine(directory,"last.png");
        using (var image=new Bitmap(24,16)) image.Save(imagePath,System.Drawing.Imaging.ImageFormat.Png);
        File.WriteAllBytes(Path.Combine(directory,"last-capture.json"),JsonSerializer.SerializeToUtf8Bytes(new CaptureOutput(imagePath,24,16,false,false,lastId),SettingsStore.Json));
        Exception? failure = null; CaptureApplication? application = null;
        var thread = new Thread(() => {
            try {
                using var app = new CaptureApplication(pipeName, directory, new NativeCaptureHandlers {
                    Scrolling = async (_, operations, id, _, _) => { Interlocked.Increment(ref scrollRuns); scrollEntered.SetResult(); if(operations.NativeJobId!=id)throw new Exception("Native job identity was not retained"); return await scrollResult.Task; },
                    Resume = (_, _, id, _, _) => { Interlocked.Increment(ref resumeRuns); return Task.FromResult<object>(new { resumed = id }); },
                    EditLast = (_, _, id, _, _) => { Interlocked.Increment(ref editRuns);return Task.FromResult<object>(new {edited=id}); },
                    DraftMatches = (_,id) => id == draftId
                });
                application = app; ready.SetResult(); Application.Run(app);
                if (app.ExitCode != 0) throw new Exception("Worker exited with an error");
            } catch (Exception error) { failure = error; ready.TrySetException(error); }
        });
        thread.SetApartmentState(ApartmentState.STA); thread.Start();
        async Task<JsonElement> Call(string method, object parameters)
        {
            using var pipe = new NamedPipeClientStream(".", pipeName, PipeDirection.InOut, PipeOptions.Asynchronous);
            await pipe.ConnectAsync(deadline.Token);
            var id = Guid.NewGuid().ToString("N");
            await PipeProtocol.WriteAsync(pipe, JsonSerializer.SerializeToUtf8Bytes(new { version = 1, id, method, @params = parameters }), deadline.Token);
            using var response = JsonDocument.Parse(await PipeProtocol.ReadAsync(pipe, deadline.Token));
            return response.RootElement.Clone();
        }
        async Task<JsonElement> Complete(string id)
        {
            while (true) {
                var job = (await Call("jobs.get", new { id })).GetProperty("result");
                if (job.GetProperty("state").GetString() != "running") return job;
                await Task.Delay(10, deadline.Token);
            }
        }
        try {
            await ready.Task.WaitAsync(deadline.Token);
            var started = (await Call("capture.unified", new { mode = "scroll", language = "en-US" })).GetProperty("result");
            await scrollEntered.Task.WaitAsync(deadline.Token);
            var again = (await Call("capture.unified", new { mode = "scroll" })).GetProperty("result");
            if (started.GetProperty("id").GetString() != again.GetProperty("id").GetString() || scrollRuns != 1)
                throw new Exception("Repeated scrolling created a second job");
            var stale = await Call("draft.resume", new { expectedId = new string('b', 32) });
            if (!stale.TryGetProperty("error", out var staleError) || staleError.ValueKind == JsonValueKind.Null)
                throw new Exception("Stale resume activated an unrelated session");
            scrollResult.SetCanceled();
            if ((await Complete(started.GetProperty("id").GetString()!)).GetProperty("state").GetString() != "cancelled")
                throw new Exception("Scrolling cancellation looked successful");
            var resume = (await Call("draft.resume", new { expectedId = draftId })).GetProperty("result");
            var resumed = await Complete(resume.GetProperty("id").GetString()!);
            if (resumed.GetProperty("state").GetString() != "completed" || resumed.GetProperty("result").GetProperty("resumed").GetString() != draftId || resumeRuns != 1)
                throw new Exception("Resume did not retain the requested draft");
            var oldImage=await Call("edit.last",new {expectedId=draftId});
            if (!oldImage.TryGetProperty("error",out var editError)||editError.ValueKind==JsonValueKind.Null||editRuns!=0) throw new Exception("Stale image reached the native editor");
            var editing=(await Call("edit.last",new {expectedId=lastId})).GetProperty("result");
            var edited=await Complete(editing.GetProperty("id").GetString()!);
            if (edited.GetProperty("state").GetString()!="completed"||edited.GetProperty("result").GetProperty("edited").GetString()!=lastId||editRuns!=1) throw new Exception("Last-image edit did not use the native entrypoint");
            await Call("shutdown", new { });
            if (!thread.Join(TimeSpan.FromSeconds(3)) || failure is not null) throw new Exception("Worker did not exit normally", failure);
            File.WriteAllText(Path.Combine(directory, "routing.json"), JsonSerializer.Serialize(new { scrollRuns, resumeRuns, editRuns, staleRejected = true, cancellationPreserved = true }));
        } finally {
            scrollResult.TrySetCanceled(); deadline.Cancel();
            if (thread.IsAlive) { application?.RequestStop(); thread.Join(TimeSpan.FromSeconds(3)); }
        }
    }
}
