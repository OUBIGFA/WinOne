using System.Diagnostics;
using System.Text.Json;
using ShareX.HelpersLib;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

internal static class NativeCaptureStartupProbe
{
    // Construct and paint the real canvas from a generated desktop-sized
    // bitmap. The window is never shown and no user pixels/input are touched.
    internal static void Run(string directory, bool prepare = false)
    {
        Exception? failure=null;
        var thread=new Thread(()=>{try{Measure(directory,prepare);}catch(Exception error){failure=error;}});
        thread.SetApartmentState(ApartmentState.STA);thread.Start();
        if(!thread.Join(TimeSpan.FromSeconds(60)))throw new TimeoutException("Capture startup probe timed out");
        if(failure is not null)throw failure;
    }
    private static void Measure(string directory, bool prepare)
    {
        Directory.CreateDirectory(directory);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);Application.EnableVisualStyles();
        var settings=new SettingsStore(directory);
        settings.Save(settings.Snapshot() with {OpenEditor=true,CopyToClipboard=false,OutputDirectory=Path.Combine(directory,"images")});
        var operations=new CaptureOperations(settings,directory,_=>throw new InvalidOperationException("Probe must not copy"));
        var preparation=Stopwatch.StartNew();
        if(prepare)NativeCaptureStandby.Prepare(operations,settings.Snapshot(),"zh-CN");
        preparation.Stop();
        if(Application.OpenForms.Count!=0)throw new InvalidOperationException("Standby retained a capture window");
        using var readyProcess=Process.GetCurrentProcess();readyProcess.Refresh();var standbyPrivateBytes=readyProcess.PrivateMemorySize64;
        var bounds=CaptureHelpers.GetScreenBounds();
        var samples=new List<double>();double processToFirstCanvas=0;
        for(int index=0;index<8;index++){
            var clock=Stopwatch.StartNew();
            try{NativeCaptureWorkflow.Run(settings,operations,"region","zh-CN",CancellationToken.None,
                acquire:()=>{var image=new Bitmap(bounds.Width,bounds.Height);using(var graphics=Graphics.FromImage(image))graphics.Clear(Color.FromArgb(80,110,140));return CapturedFrame.Own(image,bounds);},
                windows:()=>[],show:(canvas,session)=>{
                    _=canvas.Window.Handle;canvas.Window.PerformLayout();
                    using var pixel=new Bitmap(1,1);canvas.Window.DrawToBitmap(pixel,new Rectangle(0,0,1,1));
                    samples.Add(clock.Elapsed.TotalMilliseconds);
                    if(index==0)processToFirstCanvas=(DateTime.UtcNow-Process.GetCurrentProcess().StartTime.ToUniversalTime()).TotalMilliseconds;
                    canvas.Window.Close();
                }).GetAwaiter().GetResult();}
            catch(OperationCanceledException) when(samples.Count==index+1){ /* Closing the measured, untouched selection is an intentional cancellation. */ }
        }
        if(Application.OpenForms.Count!=0)throw new InvalidOperationException("Capture probe retained a window");
        using var process=Process.GetCurrentProcess();process.Refresh();
        var report=new {scope="Generated desktop-sized frame; real canvas construction and paint, no visible window or physical input-to-photon measurement",prepare,preparationMs=preparation.Elapsed.TotalMilliseconds,standbyPrivateBytes,screen=new {bounds.Width,bounds.Height},processToFirstCanvasMs=processToFirstCanvas,samplesMs=samples,privateBytes=process.PrivateMemorySize64,workingSetBytes=process.WorkingSet64,handles=process.HandleCount};
        File.WriteAllText(Path.Combine(directory,"startup.json"),JsonSerializer.Serialize(report,new JsonSerializerOptions{WriteIndented=true}));
        Console.WriteLine(JsonSerializer.Serialize(report));
    }
}
