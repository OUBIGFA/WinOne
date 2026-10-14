using System.Diagnostics;
using System.Text.Json;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

internal static class NativeLegacyDocumentTests
{
    public static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        FractionalEffectBounds(directory);
        using var image=new Bitmap(480,320);
        using(var graphics=Graphics.FromImage(image)){graphics.Clear(Color.White);graphics.FillRectangle(Brushes.Gold,30,40,300,180);}
        var marks=new AnnotationMark[] {
            new(){Kind="rectangle",X=20.5f,Y=22.25f,Width=100.5f,Height=60.5f,StrokeWidth=3.5f,Color="#0080ff"},
            new(){Kind="ellipse",X=160.25f,Y=40.5f,Width=80.5f,Height=70.25f,StrokeWidth=2.25f,Color="#ff0000"},
            new(){Kind="arrow",Points=[new(25.5f,150.25f),new(140.25f,190.5f)],StrokeWidth=4.5f,Color="#0000ff"},
            new(){Kind="pen",Points=[new(300.25f,20.5f),new(320.5f,80.25f),new(400.25f,100.5f)],StrokeWidth=2.5f,Color="#208040"},
            new(){Kind="text",X=24.5f,Y=90.25f,Width=230.5f,Height=80.5f,Text="旧稿\nLegacy",FontSize=25.5f,Color="#102030"},
            new(){Kind="mosaic",X=18.25f,Y=20.5f,Width=160.5f,Height=150.25f,EffectSize=12},
            new(){Kind="cover",X=250.25f,Y=190.5f,Width=80.5f,Height=40.25f,Color="#112233"},
            new(){Kind="blur",X=100.25f,Y=110.5f,Width=230.5f,Height=120.25f,EffectSize=2},
        };
        var document=new AnnotationDocument{Version=2,Width=image.Width,Height=image.Height,Crop=new(5,8,460,300),Marks=marks};
        var draft=new CaptureDraft(1,new string('a',32),7,image.Width,image.Height,new CaptureSettings{OutputDirectory=Path.Combine(directory,"images")},document,false,SourceKind:"scroll");
        var converted=NativeLegacyDocument.Convert(draft);
        var encoded=NativeCaptureDocumentCodec.Encode(converted,image.Size);
        var roundtrip=NativeCaptureDocumentCodec.Decode(encoded,image.Size);
        using var canvas=new WinOneCaptureCanvas(image,viewport:true);canvas.RestoreDocument(roundtrip);
        using var expected=AnnotationRenderer.Render(image,document);using var actual=canvas.RenderSelection();
        if(expected.Size!=actual.Size)throw new Exception("Legacy crop changed");
        for(var y=0;y<expected.Height;y++)for(var x=0;x<expected.Width;x++)if(expected.GetPixel(x,y)!=actual.GetPixel(x,y))throw new Exception($"Legacy pixel changed at {x},{y}");
        var exported=canvas.ExportDocument();
        if(!exported.OrderedEffects||exported.Layers.Length!=marks.Length||exported.Layers[0].LegacyStrokeWidth!=3.5f||exported.Layers[4].FontPixels!=25.5f)throw new Exception("Legacy editing data lost precision");
        using var large=new Bitmap(1920,1080);using(var graphics=Graphics.FromImage(large))graphics.Clear(Color.White);
        var largeDraft=draft with {Width=large.Width,Height=large.Height,Document=document with {Width=large.Width,Height=large.Height,Crop=null}};
        using var largeCanvas=new WinOneCaptureCanvas(large,viewport:true);largeCanvas.RestoreDocument(NativeLegacyDocument.Convert(largeDraft));
        var times=new List<double>();for(var i=0;i<5;i++){var watch=Stopwatch.StartNew();using var rendered=largeCanvas.RenderSelection();watch.Stop();times.Add(watch.Elapsed.TotalMilliseconds);}
        using var heavyCanvas=new WinOneCaptureCanvas(large,viewport:true);
        var heavy=largeDraft with {Document=largeDraft.Document with {Marks=largeDraft.Document.Marks.Append(new AnnotationMark{Kind="blur",Width=large.Width,Height=large.Height,EffectSize=64}).ToArray()}};
        heavyCanvas.RestoreDocument(NativeLegacyDocument.Convert(heavy));
        var watchHeavy=Stopwatch.StartNew();using(var rendered=heavyCanvas.RenderSelection()){}watchHeavy.Stop();
        File.WriteAllText(Path.Combine(directory,"verification.json"),JsonSerializer.Serialize(new{layers=marks.Length,pixelIdentical=true,precisionPreserved=true,renderMilliseconds=times,fullImageBlurMilliseconds=watchHeavy.Elapsed.TotalMilliseconds}));
    }

    private static void FractionalEffectBounds(string directory)
    {
        using var image=new Bitmap(12,12);
        for(var y=0;y<12;y++)for(var x=0;x<12;x++)image.SetPixel(x,y,Color.FromArgb(255,x*20,y*20,0));
        // float addition rounds this right edge to 2, while the old renderer
        // adds in double precision and includes pixel 2. Keep the same region.
        var document=new AnnotationDocument{Width=12,Height=12,Marks=[new AnnotationMark{Kind="mosaic",X=0.8f,Y=0.8f,Width=1.2f,Height=1.2f,EffectSize=2}]};
        var draft=new CaptureDraft(1,new string('b',32),0,12,12,new CaptureSettings{OutputDirectory=directory},document,false);
        using var canvas=new WinOneCaptureCanvas(image,viewport:true);canvas.RestoreDocument(NativeLegacyDocument.Convert(draft));
        using var expected=AnnotationRenderer.Render(image,document);using var actual=canvas.RenderSelection();
        File.WriteAllBytes(Path.Combine(directory,"fractional-document.json"),NativeCaptureDocumentCodec.Encode(canvas.ExportDocument(),image.Size));
        for(var y=0;y<12;y++)for(var x=0;x<12;x++)if(expected.GetPixel(x,y)!=actual.GetPixel(x,y))throw new Exception($"Fractional legacy effect changed pixel {x},{y}; origin={canvas.ViewOrigin}; bounds={canvas.ExportDocument().Layers[0].Bounds}; expected={expected.GetPixel(x,y)}, actual={actual.GetPixel(x,y)}");
        for(var i=0;i<20;i++)canvas.SetViewport(1,new PointF(i%2==0?2000.25f:-3000.5f,i%2==0?-1000.75f:4000.125f));
        canvas.AddRectangle(new Rectangle(4,4,5,5));canvas.Undo();
        using var afterPan=canvas.RenderSelection();
        for(var y=0;y<12;y++)for(var x=0;x<12;x++)if(expected.GetPixel(x,y)!=afterPan.GetPixel(x,y))throw new Exception($"Pan/history quantized old pixels at {x},{y}");
    }
}
