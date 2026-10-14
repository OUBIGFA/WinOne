using System.Diagnostics;
using System.Text.Json;
using ShareX.ScreenCaptureLib;
using WinOne.Capture.NativeUI;

internal static class NativeLegacyAsyncTests
{
    public static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        using var image = new Bitmap(1920, 1080);
        using (var graphics = Graphics.FromImage(image)) graphics.Clear(Color.Gold);
        var document = new WinOneCaptureDocument(2, image.Width, image.Height,
            new Rectangle(Point.Empty, image.Size), "select", [
                new() { Kind="rectangle", Bounds=new(20.5f,30.25f,200.5f,150.25f), StrokeColor=Color.Blue.ToArgb(), StrokeWidth=4, LegacyStrokeWidth=3.5f },
                new() { Kind="blur", Bounds=new(0,0,image.Width,image.Height), EffectSize=64, LegacyEffect=true }
            ]) { OrderedEffects=true };
        using var canvas = new WinOneCaptureCanvas(image, viewport:true);
        canvas.RestoreDocument(document);
        var copies = 0; var saves = 0;
        var saved = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        Bitmap? actual = null;
        using var session = new NativeCaptureSession(canvas, _ => copies++, output => {
            saves++; actual?.Dispose(); actual = (Bitmap)output.Clone(); saved.TrySetResult();
        });
        ToolStripItem Button(string name) => session.Toolbar.Controls.OfType<ToolStrip>()
            .SelectMany(row => row.Items.Cast<ToolStripItem>()).Single(item => item.Name == name);
        Exception? failure = null;
        using var loop = new ApplicationContext();
        var dispatched = false; var dispatchMs = 0d;
        using var deadline = new System.Windows.Forms.Timer { Interval=10000 };
        deadline.Tick += (_,_) => { failure ??= new Exception("Legacy async workflow timed out"); canvas.Window.Close(); loop.ExitThread(); };
        canvas.Window.Shown += (_,_) => canvas.Window.BeginInvoke((Action)(async () => {
            try {
                canvas.Window.Update();
                // Invalidate any first-paint cache. Save must await this edit,
                // without synchronously rendering the full-image blur.
                canvas.AddDrawing("cover", new RectangleF(400,300,80,60));
                using var expected = canvas.RenderSelection();
                canvas.AddDrawing("cover", new RectangleF(400,300,80,60));
                var watch = Stopwatch.StartNew();
                canvas.Window.BeginInvoke((Action)(() => { dispatched=true; dispatchMs=watch.Elapsed.TotalMilliseconds; }));
                Button("save").PerformClick(); Button("save").PerformClick();
                if (saves != 0) throw new Exception("Legacy save blocked the input dispatch with synchronous composition");
                await saved.Task;
                if (!dispatched || saves != 1 || copies != 0 || actual is null) throw new Exception("Legacy save did not yield or submitted more than once");
                if (!canvas.Window.IsDisposed) throw new Exception("Successful async save kept the screenshot open");
                for (var y=0;y<expected.Height;y++) for(var x=0;x<expected.Width;x++)
                    if(expected.GetPixel(x,y)!=actual.GetPixel(x,y)) throw new Exception($"Async output used stale pixels at {x},{y}");
                // Saving now completes the capture. Restore/edit cancellation
                // belongs to a new session, not the already disposed window.
                using var nextImage = new Bitmap(1920,1080);
                using (var graphics = Graphics.FromImage(nextImage)) graphics.Clear(Color.Gold);
                using var nextCanvas = new WinOneCaptureCanvas(nextImage,viewport:true);
                nextCanvas.RestoreDocument(document);
                using var nextSession = new NativeCaptureSession(nextCanvas,_=>copies++,_=>saves++);
                nextCanvas.Window.Show(); nextCanvas.Window.Update();
                nextCanvas.AddDrawing("cover", new RectangleF(600,300,80,60));
                var text = new NativeTextInputState(null,"输入中",100,100,200,40,200,24,Color.Black.ToArgb(),1,1);
                nextSession.RestoreText(text);
                if(nextSession.Snapshot()?.Text != text)throw new Exception("Pending text restoration lost its recoverable snapshot");
                await nextCanvas.PrepareRenderAsync(); await Task.Yield();
                if(nextSession.TextInput?.Text != text.Value || nextSession.TextInput.SelectionStart!=1 || nextSession.TextInput.SelectionLength!=1)
                    throw new Exception("Async text restoration changed the input or selection");
                nextSession.Toolbar.Controls.OfType<ToolStrip>().SelectMany(row=>row.Items.Cast<ToolStripItem>()).Single(item=>item.Name=="save").PerformClick();
                var pending=nextCanvas.PrepareRenderAsync();
                nextCanvas.Window.Close();
                try { await pending; } catch(ObjectDisposedException) { }
                await Task.Yield();
                if(saves!=1) throw new Exception("Closing a composing capture still saved an image");
            } catch(Exception error) { failure=error; }
            finally { if(!canvas.Window.IsDisposed)canvas.Window.Close(); loop.ExitThread(); }
        }));
        deadline.Start(); canvas.Window.Show(); Application.Run(loop); deadline.Stop();
        actual?.Dispose();
        if(failure is not null)throw failure;
        File.WriteAllText(Path.Combine(directory,"verification.json"),JsonSerializer.Serialize(new{saves,copies,dispatched,dispatchMs,latestPixels=true}));
    }
}
