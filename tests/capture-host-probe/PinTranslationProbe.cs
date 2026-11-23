using System.Diagnostics;
using System.IO.Pipes;
using System.Text.Json;
using ShareX;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

internal static class PinTranslationProbe
{
    public static int Run(string output)
    {
        Directory.CreateDirectory(output);
        var pipeName = "PinTranslationProbe-" + Guid.NewGuid().ToString("N");
        using var done = new CancellationTokenSource(TimeSpan.FromSeconds(52));
        using var parent = Process.GetCurrentProcess();
        var owner = new NativeHostConfiguration(pipeName, parent);
        int requests = 0;
        // Real named-pipe job protocol and production local OCR. Only the remote
        // service response is replaced with a known translation for this fixture.
        var service = Task.Run(async () => {
            try {
                while (!done.IsCancellationRequested) {
                    using var pipe = new NamedPipeServerStream(pipeName, PipeDirection.InOut, 1, PipeTransmissionMode.Byte, PipeOptions.Asynchronous | PipeOptions.CurrentUserOnly);
                    await pipe.WaitForConnectionAsync(done.Token);
                    using var json = JsonDocument.Parse(await PipeProtocol.ReadAsync(pipe, done.Token));
                    var req = json.RootElement;
                    object reply;
                    switch (req.GetProperty("method").GetString()) {
                        case "ai.translate.target": reply = new { target = "zh-CN" }; break;
                        case "ai.translate.start":
                            var text = req.GetProperty("params").GetProperty("text").GetString() ?? "";
                            if (!text.Contains("garden", StringComparison.OrdinalIgnoreCase) || text.Contains("example.org"))
                                throw new InvalidDataException("Source or URL protection mismatch: " + text);
                            Interlocked.Increment(ref requests);
                            reply = new { id = "fixture" }; break;
                        case "ai.jobs.get": reply = new { state = "completed", text = "欢迎来到花园。" }; break;
                        case "ai.jobs.cancel": reply = new { cancelled = true }; break;
                        default: throw new InvalidDataException("Unexpected Host request");
                    }
                    await PipeProtocol.WriteAsync(pipe, JsonSerializer.SerializeToUtf8Bytes(new { id = req.GetProperty("id").GetString(), result = reply }), done.Token);
                }
            } catch (OperationCanceledException) when (done.IsCancellationRequested) { }
        });
        bool enabled = false;
        var image = new Bitmap(520, 165);
        using (var g = Graphics.FromImage(image)) {
            g.Clear(Color.White);
            using var font = new Font("Arial", 20);
            g.DrawString("Welcome to the garden.",font,Brushes.Black,new PointF(16,20));
            g.DrawString("https://example.org",font,Brushes.Black,new PointF(16,85));
        }
        try {
            PinToScreenForm.OpenManaged(image, new PinToScreenOptions { Border = false, Shadow = false }, "en", null, () => enabled, owner);
            var pin = Application.OpenForms.OfType<PinToScreenForm>().Single();
            var strip = pin.Controls.OfType<ToolStrip>().Single();
            var action = strip.Items.OfType<ToolStripButton>().Single(item => item.AccessibleName == "Translate");
            void Wait(Func<bool> predicate) {
                while (!predicate()) {
                    done.Token.ThrowIfCancellationRequested();
                    if (service.IsFaulted) service.GetAwaiter().GetResult();
                    Application.DoEvents(); Thread.Sleep(10);
                }
            }
            if (action.Available || requests != 0) throw new Exception("Pin translation enabled by default");
            enabled = true; PinToScreenForm.RefreshManagedTranslations(); Wait(() => action.Available);
            if (requests != 0) throw new Exception("Enabling the switch started translation");
            action.PerformClick();
            var compare = pin.Controls.OfType<Button>().Single();
            Wait(() => compare.Visible && compare.Text == "Show original");
            if (requests != 1) throw new Exception("Unexpected translation count");
            using var translated = new Bitmap(pin.ClientSize.Width,pin.ClientSize.Height);
            pin.DrawToBitmap(translated,pin.ClientRectangle);
            translated.Save(Path.Combine(output,"translated.png"));
            compare.PerformClick(); Wait(() => compare.Text == "Show translation");
            using var original = new Bitmap(pin.ClientSize.Width,pin.ClientSize.Height);
            pin.DrawToBitmap(original,pin.ClientRectangle);
            original.Save(Path.Combine(output,"original.png"));
            bool changed = false;
            for (int y = 14; y < 72; y++) for (int x = 10; x < 340; x++)
                changed |= original.GetPixel(x,y) != translated.GetPixel(x,y);
            if (!changed) throw new Exception("Translation did not repaint the source text area");
            for (int y=82; y<120; y++) for(int x=20; x<300; x++)
                if (original.GetPixel(x,y) != translated.GetPixel(x,y)) throw new Exception("Protected URL changed");
            compare.PerformClick(); Wait(() => compare.Text == "Show original");
            // Partial translation uses original-image coordinates and real glyph spans.
            var chars = new[] {
                new PinGlyph("Green",new[]{10,15,45,24}),
                new PinGlyph("Garden",new[]{80,15,65,24})
            };
            var partial = PinTranslationEngine.Translate(image, new PinOcrLayout(new[] {
                new PinOcrLine("Green Garden",new[]{10,15,140,24},chars)}), new Rectangle(75,10,80,40),
                (source, _) => Task.FromResult("zh-CN"),
                (source, target, _) => source == "Garden" && target == "zh-CN" ? Task.FromResult("花园") :
                    throw new InvalidDataException("Wrong selected range: "+source), done.Token).GetAwaiter().GetResult();
            partial.Picture.Dispose();
            enabled = false; PinToScreenForm.RefreshManagedTranslations(); Wait(() => !action.Available && !compare.Visible);
            if (requests != 1) throw new Exception("Toggling caused an extra translation");
            File.WriteAllText(Path.Combine(output,"result.json"),JsonSerializer.Serialize(new { requests, defaultDisabled=true, explicitClick=true, protectedUrl=true, restored=true }));
            return 0;
        } catch(Exception e) { File.WriteAllText(Path.Combine(output,"failure.txt"),e.ToString()); throw; }
        finally { PinToScreenForm.CloseAll(); done.Cancel(); service.GetAwaiter().GetResult(); }
    }
}
