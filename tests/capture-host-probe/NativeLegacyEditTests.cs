using System.Runtime.InteropServices;
using System.Text.Json;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

internal static class NativeLegacyEditTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window,uint message,IntPtr w,IntPtr l);
    private static IntPtr At(int x,int y)=>new((y<<16)|(x&0xffff));
    internal static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        using var image=new Bitmap(640,480);using(var graphics=Graphics.FromImage(image))graphics.Clear(Color.Gold);
        var mark=new AnnotationMark{Kind="rectangle",X=100.8f,Y=80.8f,Width=120.2f,Height=60.2f,StrokeWidth=3.5f,Color="#0080ff"};
        var effect=new AnnotationMark{Kind="mosaic",X=105,Y=82,Width=100,Height=30,EffectSize=12};
        var document=new AnnotationDocument{Width=640,Height=480,Marks=[mark,effect]};
        var draft=new CaptureDraft(1,new string('c',32),0,640,480,new CaptureSettings{OutputDirectory=directory},document,false);
        using var canvas=new WinOneCaptureCanvas(image,viewport:true);canvas.RestoreDocument(NativeLegacyDocument.Convert(draft));
        void Check(AnnotationMark expectedMark) {
            using var expected=AnnotationRenderer.Render(image,document with{Marks=[expectedMark,effect]});using var actual=canvas.RenderSelection();
            if(expected.Size!=actual.Size)throw new Exception($"Legacy editing changed selection: {canvas.Selection}; output={actual.Size}; origin={canvas.ViewOrigin}; mark={expectedMark}");
            for(var y=0;y<480;y++)for(var x=0;x<640;x++)if(expected.GetPixel(x,y)!=actual.GetPixel(x,y))throw new Exception($"Legacy edit changed pixels at {x},{y}");
        }
        void Pointer(uint message,int x,int y,bool down=false)=>SendMessageW(canvas.Window.Handle,message,down?new IntPtr(1):IntPtr.Zero,At(x,y));
        Exception? failure=null;
        canvas.Window.Shown+=(_,_)=>canvas.Window.BeginInvoke((Action)(()=>{
            try {
                canvas.SetViewport(1,PointF.Empty);canvas.SelectTool("rectangle");
                Pointer(0x0200,101,100);Pointer(0x0201,101,100,true);Pointer(0x0202,101,100);
                var style=canvas.GetToolStyle();
                if(style.EffectiveValue!=3.5f)throw new Exception("Selecting old stroke lost its fractional width");
                canvas.SetToolStyle(style with{Color=Color.Black});
                if(canvas.ExportDocument().Layers[0].LegacyStrokeWidth!=3.5f)throw new Exception("Changing old stroke color rounded its width");
                var black=mark with{Color="#000000"};Check(black);
                canvas.Undo();Check(mark);canvas.Redo();Check(black);
                canvas.SelectTool("select");
                Pointer(0x0200,101,100);Pointer(0x0201,101,100,true);Pointer(0x0200,121,115,true);Pointer(0x0202,121,115);
                var moved=black with{X=black.X+20,Y=black.Y+15};Check(moved);
                canvas.Undo();Check(black);canvas.Redo();Check(moved);
            }catch(Exception error){failure=error;}finally{canvas.Window.Close();}
        }));
        canvas.ShowDialog();if(failure is not null)System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(failure).Throw();
        File.WriteAllText(Path.Combine(directory,"verification.json"),JsonSerializer.Serialize(new{colorPreservedWidth=true,moved=true,undoRedo=true,orderedPixels=true}));
    }
}
