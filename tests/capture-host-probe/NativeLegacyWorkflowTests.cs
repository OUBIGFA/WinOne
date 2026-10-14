using System.Runtime.InteropServices;
using System.Text.Json;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

internal static class NativeLegacyWorkflowTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window,uint message,IntPtr w,IntPtr l);
    internal static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        using var image=new Bitmap(480,400);using(var graphics=Graphics.FromImage(image))graphics.Clear(Color.White);
        var settings=new SettingsStore(directory);settings.Save(settings.Snapshot() with {OutputDirectory=Path.Combine(directory,"out"),OpenEditor=true});
        var source=new CaptureDraftStore(directory);var draft=source.Begin(image,settings.Snapshot(),sourceKind:"fullscreen");
        var document=draft.Document with {Marks=[
            new AnnotationMark{Kind="text",X=10.25f,Y=10.5f,Width=350.5f,Height=360.25f,FontSize=256,Text="A",Color="#102030"},
            new AnnotationMark{Kind="mosaic",X=20.8f,Y=30.8f,Width=200.2f,Height=100.2f,EffectSize=16}
        ]};
        source.Update(draft.Id,draft.Revision,document);
        var native=new NativeCaptureDraftStore(directory);
        var copies=0;var opened=0;
        using var expected=AnnotationRenderer.Render(image,document);
        var operations=new CaptureOperations(settings,directory,result=>{
            copies++;
            for(var y=0;y<400;y++)for(var x=0;x<480;x++)if(expected.GetPixel(x,y)!=result.GetPixel(x,y))throw new Exception($"Restored legacy output changed pixel {x},{y}");
        });
        Exception? failure=null;
        using var loop=new ApplicationContext();
        using var start=new System.Windows.Forms.Timer{Interval=1};
        using var deadline=new System.Windows.Forms.Timer{Interval=10000};
        deadline.Tick+=(_,_)=>{failure??=new Exception("Legacy resume workflow timed out");foreach(var form in Application.OpenForms.Cast<Form>().ToArray())form.Close();loop.ExitThread();};
        start.Tick+=async(_,_)=>{
            start.Stop();
            try {
                await operations.MigrateLegacyAsync(native);
                await NativeCaptureWorkflow.Resume(settings,operations,draft.Id,"zh-CN",CancellationToken.None,native,(canvas,session)=>{
                    opened++;
                    if(!canvas.PrepareRenderAsync().IsCompletedSuccessfully)throw new Exception("Legacy draft opened before its first annotated frame was ready");
                    canvas.Window.Shown+=(_,_)=>canvas.Window.BeginInvoke((Action)(()=>SendMessageW(canvas.Window.Handle,0x0203,IntPtr.Zero,IntPtr.Zero)));
                    canvas.ShowDialog();
                });
                if(opened!=1||copies!=1||native.Read() is not null||source.Read() is not null)throw new Exception("Legacy resume did not complete exactly once");
            }catch(Exception error){failure=error;}finally{loop.ExitThread();}
        };
        start.Start();deadline.Start();Application.Run(loop);deadline.Stop();
        if(failure is not null)System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(failure).Throw();
        File.WriteAllText(Path.Combine(directory,"verification.json"),JsonSerializer.Serialize(new{opened,copies,fontPixels=256,firstFrameReady=true,retired=true,pixelIdentical=true}));
    }
}
