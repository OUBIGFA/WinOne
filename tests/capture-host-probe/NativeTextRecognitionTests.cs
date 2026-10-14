using ShareX.ScreenCaptureLib;
using WinOne.Capture.NativeUI;
using System.Runtime.InteropServices;

internal static class NativeTextRecognitionTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window,uint message,IntPtr w,IntPtr l);
    private static IntPtr At(int x,int y)=>new((y<<16)|(x&0xffff));
    internal static void Run()
    {
        foreach(var mode in new[]{"ocr-window","translation-window","translation-shortcut","opening-failure","cancel-opening","silent-ocr","silent-failure","clipboard-failure","retry-opening"})RunMode(mode);
    }
    private static void RunMode(string mode)
    {
        var silent=mode is "silent-ocr" or "silent-failure" or "clipboard-failure";
        var translate=mode.StartsWith("translation",StringComparison.Ordinal);
        using var image=new Bitmap(800,500);using(var graphics=Graphics.FromImage(image))graphics.Clear(Color.White);
        using var canvas=new WinOneCaptureCanvas(image);using var service=new TextService(mode);
        var copied="";var hidden=false;var toolbarFlashed=false;var closed=false;var started=false;var verified=false;var retry=false;
        using var session=new NativeCaptureSession(canvas,_=>throw new Exception("Text path copied an image"),_=>{},textService:service,
            textMode:silent?"ocr":mode=="translation-shortcut"?"translate":null,
            copyText:value=>{if(mode=="clipboard-failure")throw new InvalidOperationException("Clipboard busy");copied=value;});
        canvas.Window.VisibleChanged+=(_,_)=>{if(!canvas.Window.Visible&&session.RecognitionBusy)hidden=true;};
        canvas.Window.FormClosed+=(_,_)=>closed=true;
        session.Toolbar.VisibleChanged+=(_,_)=>{if(mode=="silent-ocr"&&session.Toolbar.Visible)toolbarFlashed=true;};
        Exception? failure=null;var deadline=DateTime.UtcNow.AddSeconds(7);
        void Launch()=>session.Toolbar.Controls.OfType<ToolStrip>().SelectMany(row=>row.Items.Cast<ToolStripItem>()).Single(item=>item.Name==(translate?"translate":"ocr")).PerformClick();
        using var timer=new System.Windows.Forms.Timer{Interval=20};
        timer.Tick+=(_,_)=>{
            try{
                if(DateTime.UtcNow>deadline)throw new Exception("Text operation timed out: "+mode);
                if(!session.Ready)return;
                if(!started){started=true;
                    SendMessageW(canvas.Window.Handle,0x0200,IntPtr.Zero,At(100,100));SendMessageW(canvas.Window.Handle,0x0201,new IntPtr(1),At(100,100));
                    SendMessageW(canvas.Window.Handle,0x0200,new IntPtr(1),At(400,300));SendMessageW(canvas.Window.Handle,0x0202,IntPtr.Zero,At(400,300));
                    if(!silent&&mode!="translation-shortcut")Launch();return;
                }
                if(mode=="cancel-opening"&&service.Opens>0&&!verified){
                    SendMessageW(canvas.Window.Handle,0x0201,new IntPtr(1),At(50,50));SendMessageW(canvas.Window.Handle,0x0202,IntPtr.Zero,At(50,50));
                    if(session.CloseRequested)throw new Exception("Cancel committed an opening");verified=true;
                }
                if(session.RecognitionBusy)return;
                if(mode=="retry-opening"&&!retry&&session.Error is not null){retry=true;Launch();return;}
                if(mode is "opening-failure" or "silent-failure" or "clipboard-failure" or "cancel-opening"){
                    if(!canvas.Window.Visible||session.CloseRequested||session.Copied||session.ResultPresented)throw new Exception("Failed action discarded the capture");
                    if(mode!="cancel-opening"&&string.IsNullOrEmpty(session.Error))throw new Exception("Failed action gave no feedback");
                    if(canvas.Selection is not {Width:301,Height:201})throw new Exception("Failed action changed the selection");
                    verified=true;canvas.Window.Close();
                }
            }catch(Exception error){failure=error;canvas.Window.Close();}
        };
        timer.Start();Application.Run(canvas.Window);timer.Stop();if(failure is not null)throw failure;
        if(!closed)throw new Exception("Capture remained open");
        if(mode=="silent-ocr"){
            if(copied!="原文\nHello"||!session.Copied||session.ResultPresented||!hidden||toolbarFlashed||service.Opens!=0)throw new Exception("Silent OCR behavior changed");
        }else if(mode is "opening-failure" or "silent-failure" or "clipboard-failure" or "cancel-opening"){
            if(!verified||copied!="")throw new Exception("Failure was not preserved");
        }else{
            if(!session.ResultPresented||session.Copied||copied!=""||session.ResultWindowId!="owned-result")throw new Exception("Visible result did not close the canvas independently");
            // Both pointer endpoints are selected pixels, matching the canvas contract.
            if(service.Calls!=0||service.Transfer is null||service.Transfer.Size!=new Size(301,201)||service.Translation!=translate)throw new Exception($"Result handoff {mode}: recognition calls={service.Calls}, size={service.Transfer?.Size}, translation={service.Translation}, expected translation={translate}");
            if(service.Opens!=(mode=="retry-opening"?2:1))throw new Exception("Duplicate result opening");
            if(service.Transfer.GetPixel(1,1).ToArgb()!=Color.White.ToArgb())throw new Exception("Receiver image was lost when the canvas closed");
        }
        if(silent&&service.Calls!=1)throw new Exception("Silent OCR started more than once");
    }
    private sealed class TextService(string mode):ICaptureTextService,IDisposable
    {
        internal int Calls,Opens;internal bool Translation;internal Bitmap? Transfer;
        public async Task<string> Recognize(Bitmap image,CancellationToken token){Calls++;await Task.Delay(100,token);if(mode=="silent-failure")throw new InvalidOperationException("ocrEmpty");return "原文\nHello";}
        public Task<string> Translate(string text,CancellationToken token)=>throw new Exception("Translation must run outside the capture lifetime");
        public async Task<string> OpenResult(Bitmap image,bool translate,CancellationToken token){
            Opens++;Translation=translate;
            if(mode=="cancel-opening"){await Task.Delay(Timeout.Infinite,token);throw new Exception("Cancelled opening resumed");}
            await Task.Delay(80,token);
            if(mode=="opening-failure"||mode=="retry-opening"&&Opens==1)throw new InvalidOperationException("resultUnavailable");
            Transfer=(Bitmap)image.Clone();return "owned-result";
        }
        public void Dispose()=>Transfer?.Dispose();
    }
}
