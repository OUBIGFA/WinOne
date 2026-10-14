using System.Diagnostics;
using System.Reflection;
using System.Runtime.InteropServices;
using System.Text.Json;
using ShareX.ScreenCaptureLib;

internal static class NativeLegacyRenderLifetimeTests
{
    [DllImport("user32.dll")] private static extern int GetGuiResources(IntPtr process,int flags);
    public static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        var type=typeof(WinOneCaptureCanvas).Assembly.GetType("ShareX.ScreenCaptureLib.WinOneCompositionJob",throwOnError:true)!;
        var completion=type.GetProperty("Completion",BindingFlags.Instance|BindingFlags.NonPublic)!;
        var error=type.GetProperty("Error",BindingFlags.Instance|BindingFlags.NonPublic)!;
        var document=new WinOneCaptureDocument(2,320,180,new Rectangle(0,0,320,180),"select",[
            new(){Kind="blur",Bounds=new(0,0,320,180),LegacyEffect=true,EffectSize=64}
        ]){OrderedEffects=true};
        var samples=new List<object>();
        using var process=Process.GetCurrentProcess();
        var baseline=0;var final=0;
        for(var i=0;i<100;i++) {
            var image=new Bitmap(320,180);
            var job=(IDisposable)Activator.CreateInstance(type,BindingFlags.Instance|BindingFlags.NonPublic,null,[0L,image,document],null)!;
            var done=(Task)completion.GetValue(job)!;
            if(i%2==0)job.Dispose();
            if(!done.Wait(TimeSpan.FromSeconds(5)))throw new Exception("Detached render did not release its inputs");
            if(error.GetValue(job) is Exception failure)throw failure;
            job.Dispose();
            // No window or message loop consumes this result: disposal itself
            // must own the input/output, including completion after cancellation.
            try { _=image.GetPixel(0,0); throw new Exception("Abandoned rendering bitmap remained alive"); }
            catch(ArgumentException) { }
            if(i==19)baseline=GetGuiResources(process.Handle,0);
            if(i>=19 && i%20==19){process.Refresh();final=GetGuiResources(process.Handle,0);samples.Add(new{iteration=i+1,gdi=final,handles=process.HandleCount,privateBytes=process.PrivateMemorySize64});}
        }
        if(final>baseline+2)throw new Exception("Detached rendering retained GDI objects");
        File.WriteAllText(Path.Combine(directory,"verification.json"),JsonSerializer.Serialize(new{iterations=100,cancelled=50,unclaimed=50,allBitmapsDisposed=true,samples}));
    }
}
