using System.Text.Json;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

internal static class NativeLegacyMigrationTests
{
    public static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        LargeDraft(Path.Combine(directory,"large"));
        using var image=new Bitmap(200,140);using(var graphics=Graphics.FromImage(image))graphics.Clear(Color.Gold);
        var settings=new CaptureSettings{OutputDirectory=Path.Combine(directory,"output")};
        var legacy=new CaptureDraftStore(directory);var original=legacy.Begin(image,settings,sourceKind:"scroll");
        var document=original.Document with {Marks=[new AnnotationMark{Kind="rectangle",X=20.5f,Y=20.25f,Width=100.5f,Height=70.5f,StrokeWidth=3.5f}]};
        original=legacy.Update(original.Id,original.Revision,document);
        var failing=new NativeCaptureDraftStore(directory,(_,_)=>throw new IOException("migration encode failed"));
        try{legacy.MigrateToNative(failing);throw new Exception("Failed migration returned success");}catch(IOException error)when(error.Message=="migration encode failed"){}
        if(legacy.Read()?.Id!=original.Id||failing.Read() is not null)throw new Exception("Failed migration lost the original");
        // Native publication succeeds, but retirement cannot yet be written.
        // The next attempt must reuse that exact native image and identity.
        var retirement=Path.Combine(directory,"retired-drafts.json");Directory.CreateDirectory(retirement);
        var writes=0;var native=new NativeCaptureDraftStore(directory,(pixels,path)=>{writes++;PngFiles.Write(pixels,path);});
        try{legacy.MigrateToNative(native);throw new Exception("Retirement failure was hidden");}catch(Exception error)when(error is IOException or UnauthorizedAccessException){}
        if(native.Read()?.Id!=original.Id||legacy.Read()?.Id!=original.Id)throw new Exception("Interrupted migration did not retain both copies");
        Directory.Move(retirement,retirement+"-blocked");
        if(legacy.MigrateToNative(native)!=original.Id||legacy.Read() is not null||writes!=1)throw new Exception("Migration retry recaptured or duplicated the image");
        var recovered=native.Recover()??throw new Exception("Migrated draft was not recoverable");
        using(var originalOutput=AnnotationRenderer.Render(image,document))
        using(var canvas=new WinOneCaptureCanvas(image,viewport:true)) {
            canvas.RestoreDocument(recovered.Document);using var output=canvas.RenderSelection();
            for(var y=0;y<output.Height;y++)for(var x=0;x<output.Width;x++)if(output.GetPixel(x,y)!=originalOutput.GetPixel(x,y))throw new Exception("Migration changed pixels");
            var edited=canvas.ExportDocument();
            native.Update(recovered.Id,recovered.Revision,edited);
            if(native.Recover()!.Document.Layers[0].LegacyStrokeWidth!=3.5f)throw new Exception("Saving lost the migrated stroke precision");
        }
        File.WriteAllText(Path.Combine(directory,"verification.json"),JsonSerializer.Serialize(new{sameIdentity=true,encodeFailurePreserved=true,interruptionRecovered=true,writes,pixelIdentical=true}));
        var metadata=Path.Combine(directory,"native-capture","current.json");
        var valid=File.ReadAllText(metadata);
        foreach(var malformed in new[]{"",new string('g',64),new string('a',63)}) {
            var json=System.Text.Json.Nodes.JsonNode.Parse(valid)!;json["legacySource"]=malformed;
            File.WriteAllText(metadata,json.ToJsonString());
            try { native.Read(); throw new Exception("Invalid migration fingerprint was accepted"); }
            catch(InvalidDataException) { }
        }
        File.WriteAllText(metadata,valid);
    }

    private static void LargeDraft(string directory)
    {
        Directory.CreateDirectory(directory);
        using var image=new Bitmap(20,20);
        var old=new CaptureDraftStore(directory);
        var draft=old.Begin(image,new CaptureSettings{OutputDirectory=Path.Combine(directory,"out")});
        var marks=Enumerable.Range(0,512).Select(_=>new AnnotationMark{Kind="text",Width=10,Height=10,Text=new string('x',1200)}).ToArray();
        old.Update(draft.Id,draft.Revision,draft.Document with {Marks=marks});
        var oldBytes=new FileInfo(Path.Combine(directory,"capture-draft.json")).Length;
        var native=new NativeCaptureDraftStore(directory);
        old.MigrateToNative(native);
        var recovered=native.Recover()??throw new Exception("Large valid legacy draft was lost");
        if(recovered.Document.Layers.Length!=512||recovered.Document.Layers.Any(layer=>layer.Text.Length!=1200))throw new Exception("Large legacy draft was truncated");
        File.WriteAllText(Path.Combine(directory,"verification.json"),JsonSerializer.Serialize(new{oldBytes,nativeBytes=NativeCaptureDocumentCodec.Encode(recovered.Document,image.Size).Length,layers=512}));
    }
}
