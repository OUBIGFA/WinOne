using WinOne.Capture;
using ShareX.ScreenCaptureLib;

internal static class NativeDraftReplacementTests
{
    public static void Run(string directory) {
        Directory.CreateDirectory(directory);
        using var oldImage = new Bitmap(80, 160);
        using var newImage = new Bitmap(90, 180);
        using (var g = Graphics.FromImage(oldImage)) g.Clear(Color.Blue);
        using (var g = Graphics.FromImage(newImage)) g.Clear(Color.Gold);
        using var oldCanvas = new WinOneCaptureCanvas(oldImage, viewport:true);
        oldCanvas.SetRegion(new Rectangle(Point.Empty, oldImage.Size));
        oldCanvas.AddRectangle(new Rectangle(10, 20, 30, 40));
        using var newCanvas = new WinOneCaptureCanvas(newImage, viewport:true);
        newCanvas.SetRegion(new Rectangle(Point.Empty, newImage.Size));
        using var ordinary = new WinOneCaptureCanvas(oldImage);
        var originalWindow = ordinary.Window.Handle;
        using(var prepared = WinOneCanvasReplacement.Prepare(newImage)) ordinary.ApplyReplacement(prepared);
        if(!ordinary.HasViewport || ordinary.Window.Handle != originalWindow || ordinary.Selection?.Size != newImage.Size)
            throw new Exception("Entering scrolling mode replaced the window or lost the long image viewport");
        var settings = new CaptureSettings { OutputDirectory=Path.Combine(directory, "output") };
        var store = new NativeCaptureDraftStore(directory);
        var original = store.Begin(oldImage, settings, oldCanvas.ExportDocument(), "region", CapturePlacement.From(new Rectangle(Point.Empty,oldImage.Size),new Rectangle(Point.Empty,oldImage.Size)));
        var failing = new NativeCaptureDraftStore(directory, (_, _) => throw new IOException("encode rejected"));
        try { failing.ReplaceScroll(original.Id, original.Revision, newImage, settings, newCanvas.ExportDocument()); throw new Exception("Encoding failure accepted"); }
        catch (IOException e) when(e.Message=="encode rejected") { }
        if (store.Read()?.Id != original.Id) throw new Exception("Failed replacement lost old draft");
        using var cancelled=new CancellationTokenSource();
        var cancelling=new NativeCaptureDraftStore(directory,(image,path)=>{PngFiles.Write(image,path);cancelled.Cancel();});
        try {cancelling.ReplaceScroll(original.Id,original.Revision,newImage,settings,newCanvas.ExportDocument(),cancellation:cancelled.Token);throw new Exception("Cancelled encoding committed a retake");}
        catch(OperationCanceledException) when(cancelled.IsCancellationRequested) { }
        if(store.Read()?.Id!=original.Id)throw new Exception("Cancellation during encoding replaced the old draft");
        using (var retained=store.LoadImage(original.Id,original.Revision))
            if(retained.GetPixel(0,0)!=Color.FromArgb(Color.Blue.ToArgb())) throw new Exception("Old pixels changed");
        var revised=store.Update(original.Id, original.Revision, oldCanvas.ExportDocument());
        try { store.ReplaceScroll(original.Id, original.Revision, newImage, settings, newCanvas.ExportDocument()); throw new Exception("Stale replacement accepted"); }
        catch(InvalidOperationException) { }
        var replacement=store.ReplaceScroll(revised.Id,revised.Revision,newImage,settings,newCanvas.ExportDocument(),"scrollStopped");
        var recovered=new NativeCaptureDraftStore(directory).Recover()!;
        if(recovered.Id!=replacement.Id || recovered.Id==original.Id || recovered.Document.Layers.Length!=0 || recovered.Notice!="scrollStopped") throw new Exception("Replacement metadata incorrect");
        using var pixels=store.LoadImage(recovered.Id,recovered.Revision);
        if(pixels.Size!=newImage.Size || pixels.GetPixel(0,0).ToArgb()!=Color.Gold.ToArgb()) throw new Exception("Replacement pixels incorrect");
        Console.WriteLine("PASS: scrolling draft replacement, encode failure preservation, stale revision rejection and recovery");
    }
}
