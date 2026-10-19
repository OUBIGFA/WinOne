using System.Runtime.InteropServices;
using System.Text.Json;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

internal static class NativeRetakeWorkflowTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window,uint message,IntPtr w,IntPtr l);
    [DllImport("user32.dll")] private static extern uint GetGuiResources(IntPtr process,uint flags);
    public static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        var iterations=Environment.GetEnvironmentVariable("WINONE_RETAKE_ITERATIONS") is { } count?int.Parse(count,System.Globalization.CultureInfo.InvariantCulture):1;
        if(iterations is <1 or >100)throw new ArgumentOutOfRangeException(nameof(iterations));
        var resources=new List<object>();var timings=new List<double>();
        using var process=System.Diagnostics.Process.GetCurrentProcess();
        var settings=new SettingsStore(directory);
        settings.Save(settings.Snapshot() with {OpenEditor=true,CopyToClipboard=false,OutputDirectory=Path.Combine(directory,"images")});
        var failWrite=0; var encodes=0;
        var drafts=new NativeCaptureDraftStore(directory,(image,path)=>{
            Interlocked.Increment(ref encodes);
            if(Volatile.Read(ref failWrite)!=0)throw new IOException("retake encode failed");
            PngFiles.Write(image,path);
        });
        Bitmap? copied=null;var copies=0;
        var operations=new CaptureOperations(settings,directory,image=>{copies++;copied=new Bitmap(image);});
        operations.NativeJobId="retake-test-job";
        using var original=new Bitmap(240,1200);using(var graphics=Graphics.FromImage(original))graphics.Clear(Color.AliceBlue);
        using var replacement=new Bitmap(200,1800);using(var graphics=Graphics.FromImage(replacement))graphics.Clear(Color.Gold);
        var sources=new List<ScrollingFrameTests.Source>();
        for(var i=0;i<4;i++) {
            var source=new ScrollingFrameTests.Source(new Bitmap(replacement)){Selected=i!=0};
            if(i==1)source.Completion.SetException(new IOException("retake stitch failed"));
            else source.Completion.SetResult(ScrollingCaptureStatus.Successful);
            sources.Add(source);
        }
        sources[3].Started=()=>{if(!operations.StopScrolling("retake-test-job"))throw new Exception("Retake did not retain the externally visible job ID");};
        var nextSource=0;var progress=new List<ScrollingFrameTests.ProgressLease>();
        Bitmap? expected=null;string? oldId=null;string? newId=null;long window=0;
        using var stopping=new CancellationTokenSource();Exception? failure=null;
        try {
            using var frame=CapturedFrame.Own(new Bitmap(original),new Rectangle(Point.Empty,original.Size));
            try {
                NativeCaptureWorkflow.EditScrollingFrame(frame,settings.Snapshot(),operations,null,"en-US",stopping.Token,drafts,
                    show:(canvas,session)=>{
                        window=canvas.Window.Handle.ToInt64();
                        using var deadline=new System.Windows.Forms.Timer{Interval=iterations>1?45000:15000};
                        deadline.Tick+=(_,_)=>{failure=new TimeoutException("Retake workflow did not finish");stopping.Cancel();};
                        canvas.Window.Shown+=async (_,_)=>{
                            // Hide/Show during retake raises Shown only once for
                            // this form, but the guard also protects test reentry.
                            if(nextSource!=0)return;
                            try {
                                canvas.AddRectangle(new Rectangle(20,900,100,80));
                                using var before=canvas.RenderSelection();
                                var document=JsonSerializer.Serialize(canvas.ExportDocument());
                                var button=session.Toolbar.Controls.OfType<ToolStrip>().SelectMany(row=>row.Items.Cast<ToolStripItem>()).Single(item=>item.Name=="retake");
                                for(var attempt=0;attempt<3;attempt++) {
                                    Volatile.Write(ref failWrite,attempt==2?1:0);
                                    button.PerformClick();await session.RetakeCompletion.WaitAsync(TimeSpan.FromSeconds(8));
                                    if(session.Retaking||canvas.Window.Handle.ToInt64()!=window||JsonSerializer.Serialize(canvas.ExportDocument())!=document)
                                        throw new Exception("Failed retake replaced the canvas or its annotations");
                                    var retained=drafts.Recover()??throw new Exception("Failed retake lost the saved draft");
                                    oldId??=retained.Id;if(retained.Id!=oldId)throw new Exception("Failed retake changed draft identity");
                                    using var unchanged=canvas.RenderSelection();AssertPixels(before,unchanged);
                                    if(copies!=0)throw new Exception("Failed retake changed the clipboard");
                                }
                                Volatile.Write(ref failWrite,0);
                                button.PerformClick();await session.RetakeCompletion.WaitAsync(TimeSpan.FromSeconds(8));
                                var replaced=drafts.Recover()??throw new Exception("Successful retake was not saved");newId=replaced.Id;
                                if(nextSource!=4||newId==oldId||canvas.Window.Handle.ToInt64()!=window||canvas.AnnotationCount!=0||canvas.Selection!=new Rectangle(Point.Empty,replacement.Size))
                                    throw new Exception("Successful retake did not replace the same canvas cleanly");
                                canvas.Undo();using(var current=canvas.RenderSelection())AssertPixels(replacement,current);
                                for(var iteration=1;iteration<iterations;iteration++) {
                                    canvas.AddRectangle(new Rectangle(20,1500,90,80));
                                    var watch=System.Diagnostics.Stopwatch.StartNew();
                                    button.PerformClick();await session.RetakeCompletion.WaitAsync(TimeSpan.FromSeconds(8));watch.Stop();
                                    timings.Add(watch.Elapsed.TotalMilliseconds);
                                    if(canvas.Window.Handle.ToInt64()!=window||canvas.AnnotationCount!=0||canvas.Selection!=new Rectangle(Point.Empty,replacement.Size))throw new Exception("Repeated retake changed window identity or retained old annotations");
                                    if((iteration+1)%10==0||iteration==iterations-1) {
                                        process.Refresh();resources.Add(new{iteration=iteration+1,handles=process.HandleCount,gdi=GetGuiResources(process.Handle,0),user=GetGuiResources(process.Handle,1),privateBytes=process.PrivateMemorySize64,workingSet=process.WorkingSet64});
                                    }
                                }
                                canvas.AddRectangle(new Rectangle(20,1500,90,80));expected=canvas.RenderSelection();
                                stopping.Cancel();
                            } catch(Exception error){failure=error;stopping.Cancel();}
                        };
                        using var context=new ApplicationContext(canvas.Window);
                        deadline.Start();Application.Run(context);deadline.Stop();
                    },retakeSource:_=>{
                        if(nextSource==sources.Count){var source=new ScrollingFrameTests.Source(new Bitmap(replacement));source.Completion.SetResult(ScrollingCaptureStatus.Successful);sources.Add(source);}
                        return sources[nextSource++];
                    },retakeProgress:_=>{var lease=new ScrollingFrameTests.ProgressLease();progress.Add(lease);return lease;}).GetAwaiter().GetResult();
                throw new Exception("Interrupted final editing returned success");
            } catch(OperationCanceledException) when(stopping.IsCancellationRequested) { }
            if(failure is not null)throw failure;
            if(sources.Any(source=>!source.Disposed)||progress.Any(item=>!item.Disposed))throw new Exception("Retake retained acquisition resources");
            var resumed=drafts.Recover()??throw new Exception("Retaken image was not recoverable");
            if(resumed.Id==oldId||resumed.Document.Layers.Length!=1)throw new Exception("Retake recovery used the old image or lost new edits");
            NativeCaptureWorkflow.Resume(settings,operations,resumed.Id,"en-US",CancellationToken.None,drafts,show:(canvas,session)=>{
                canvas.Window.Shown+=(_,_)=>canvas.Window.BeginInvoke((Action)(()=>NativeScrollInteractionTests.Confirm(session)));
                canvas.ShowDialog();
            }).GetAwaiter().GetResult();
            if(copied is null||expected is null||copies!=1||drafts.Recover() is not null)throw new Exception("Retake recovery did not complete once");
            AssertPixels(expected,copied);
            File.WriteAllText(Path.Combine(directory,"verification.json"),JsonSerializer.Serialize(new {sameWindow=true,cancelPreserved=true,stitchFailurePreserved=true,encodeFailurePreserved=true,newEditsRecovered=true,iterations,encodes,copies,timings,resources}));
        } finally {foreach(var source in sources)source.Dispose();expected?.Dispose();copied?.Dispose();}
        VerifyModelessLifetime(Path.Combine(directory,"modeless"));
        Console.WriteLine("PASS: native retake preserves failures, replaces the same canvas and recovers new annotations");
    }
    private static void VerifyModelessLifetime(string directory)
    {
        Directory.CreateDirectory(directory);
        using var context=new ApplicationContext();
        using var start=new System.Windows.Forms.Timer{Interval=1};
        using var cancellation=new CancellationTokenSource();
        Exception? failure=null;
        start.Tick+=async (_,_)=>{
            start.Stop();
            var store=new SettingsStore(directory);var drafts=new NativeCaptureDraftStore(directory);
            var operations=new CaptureOperations(store,directory,_=>throw new Exception("Modeless lifetime check copied an image"));
            using var image=new Bitmap(80,400);using var replacement=new Bitmap(120,700);
            using var frame=CapturedFrame.Own(new Bitmap(image),new Rectangle(Point.Empty,image.Size));
            using var source=new ScrollingFrameTests.Source(new Bitmap(replacement));
            var entered=new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
            source.Started=()=>entered.TrySetResult();
            Task<object>? operation=null;
            try {
                operation=NativeCaptureWorkflow.EditScrollingFrame(frame,store.Snapshot(),operations,null,"en-US",cancellation.Token,drafts,
                    retakeSource:_=>source,retakeProgress:_=>new ScrollingFrameTests.ProgressLease());
                var window=Application.OpenForms.OfType<RegionCaptureForm>().Single();var handle=window.Handle;
                var toolbar=window.Controls.OfType<NativeToolbarControl>().Single();
                toolbar.Controls.OfType<ToolStrip>().SelectMany(row=>row.Items.Cast<ToolStripItem>()).Single(item=>item.Name=="retake").PerformClick();
                await entered.Task.WaitAsync(TimeSpan.FromSeconds(5));
                if(operation.IsCompleted||window.Visible)throw new Exception("Hiding the editor completed the capture job");
                source.Completion.SetResult(ScrollingCaptureStatus.Successful);
                var deadline=DateTime.UtcNow.AddSeconds(5);
                while(!window.Visible||!toolbar.Enabled) {
                    if(DateTime.UtcNow>=deadline)throw new TimeoutException("Modeless retake did not return to editing");
                    await Task.Delay(10);
                }
                if(operation.IsCompleted||window.Handle!=handle||drafts.Recover()?.Document.Height!=700)throw new Exception("Modeless retake lost the live session");
                SendMessageW(window.Handle,0x0100,new IntPtr((int)Keys.Escape),IntPtr.Zero);
                try {await operation.WaitAsync(TimeSpan.FromSeconds(5));throw new Exception("Cancel returned a completed capture");}
                catch(OperationCanceledException) { }
                if(!window.IsDisposed||drafts.Recover() is not null)throw new Exception("Modeless close left a window or draft");
                File.WriteAllText(Path.Combine(directory,"verification.json"),"{\"modelessLifetime\":true,\"sameWindow\":true,\"normalCancel\":true}");
            } catch(Exception error){failure=error;}
            finally {
                cancellation.Cancel();source.Completion.TrySetCanceled();
                if(operation is not null&&!operation.IsCompleted) {try{await operation.WaitAsync(TimeSpan.FromSeconds(3));}catch(Exception) { }}
                context.ExitThread();
            }
        };
        start.Start();Application.Run(context);
        if(failure is not null)throw failure;
    }
    private static void AssertPixels(Bitmap expected,Bitmap actual)
    {
        if(expected.Size!=actual.Size)throw new Exception("Retake image dimensions changed");
        for(var y=0;y<expected.Height;y++)for(var x=0;x<expected.Width;x++)if(expected.GetPixel(x,y)!=actual.GetPixel(x,y))throw new Exception($"Retake pixel changed at {x},{y}");
    }
}
