using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

// A visible result is handed to WinOne's independent result host. Once that
// window confirms ownership, this canvas closes instead of remaining hidden.
internal sealed class NativeRecognitionSession : IDisposable
{
    private readonly WinOneCaptureCanvas canvas;
    private readonly ICaptureTextService service;
    private readonly Action<string> clipboard;
    private readonly Action<bool> complete;
    private readonly bool english;
    private CancellationTokenSource? request;
    private bool disposed, silent, completed;
    internal bool Busy { get; private set; }
    internal bool Active => Busy || silent;
    internal bool Silent => silent;
    internal string? Error { get; private set; }
    internal string? WindowID { get; private set; }
    internal event Action? Changed;
    internal event Action? ReturnedToCapture;
    internal NativeRecognitionSession(WinOneCaptureCanvas canvas, ICaptureTextService service,
        Action<string> clipboard, Action<bool> complete, bool english)
    {
        this.canvas=canvas;this.service=service;this.clipboard=clipboard;this.complete=complete;this.english=english;
        canvas.Window.FormClosed+=Closed;
    }
    private string T(string chinese,string translated)=>english?translated:chinese;
    private void Closed(object? sender,FormClosedEventArgs args)=>Cancel();
    internal void Cancel(){request?.Cancel();Busy=false;}
    internal void Dismiss(){Cancel();silent=false;Error=null;ReturnedToCapture?.Invoke();Changed?.Invoke();if(!canvas.Window.IsDisposed)canvas.Window.Focus();}
    internal void RestoreAfterCloseFailure(){silent=false;ReturnedToCapture?.Invoke();Changed?.Invoke();}
    internal void Start(bool translate,bool copyAndExit)
    {
        if(disposed||completed||canvas.Selection is null)return;
        Cancel();silent=copyAndExit;Error=null;request=new CancellationTokenSource();Busy=true;
        Changed?.Invoke();_ = Run(request,translate,copyAndExit);
    }
    private async Task Run(CancellationTokenSource lifetime,bool translate,bool copyAndExit)
    {
        try {
            canvas.FinishGesture();await canvas.PrepareRenderAsync(lifetime.Token);
            using var image=canvas.RenderSelection();
            if(copyAndExit){
                // Keep the existing silent OCR shortcut. No result window is
                // opened, and the captured image is owned before hiding.
                canvas.Window.Hide();
                var text=await service.Recognize(image,lifetime.Token);
                if(string.IsNullOrWhiteSpace(text))throw new InvalidOperationException("ocrEmpty");
                if(!Current(lifetime))return;
                try{clipboard(text);}catch(Exception error){throw new InvalidOperationException("clipboardFailed",error);}
                completed=true;complete(true);
            } else {
                // The receiver owns a copy before it acknowledges its window.
                // No OCR or AI request remains attached to this canvas lifetime.
                var windowId=await service.OpenResult(image,translate,lifetime.Token);
                if(!Current(lifetime))return;
                WindowID=windowId;
                completed=true;complete(false);
            }
        } catch(OperationCanceledException)when(lifetime.IsCancellationRequested){}
        catch(Exception error){
            if(Current(lifetime)){
                silent=false;Error=Explain(error.Message);ReturnedToCapture?.Invoke();
                if(!canvas.Window.Visible)canvas.Window.Show();
            }
        } finally {
            if(ReferenceEquals(request,lifetime)){request=null;Busy=false;if(!disposed&&!canvas.Window.IsDisposed)Changed?.Invoke();}
            lifetime.Dispose();
        }
    }
    private bool Current(CancellationTokenSource lifetime)=>!disposed&&!canvas.Window.IsDisposed&&ReferenceEquals(request,lifetime)&&!lifetime.IsCancellationRequested;
    private string Explain(string code)=>code switch {
        "ocrEmpty"=>T("未识别到文字，请重新选择区域。","No text found. Select another area."),
        "ocrUnavailable"=>T("OCR 文件缺失，请修复安装。","OCR files are missing. Repair the installation."),
        "ocrTimeout"=>T("识别超时，请缩小选区后重试。","Recognition timed out. Select a smaller area and retry."),
        "clipboardFailed"=>T("复制失败，请打开 OCR 结果后重试复制。","Copy failed. Open the OCR result and try copying again."),
        "resultUnavailable"=>T("结果界面无法打开，请修复安装后重试。","The result window is unavailable. Repair the installation and retry."),
        _=>T("操作未完成，请重试；仍失败时查看截图日志。","The action did not finish. Retry or check the capture log.")
    };
    public void Dispose(){if(disposed)return;disposed=true;Cancel();canvas.Window.FormClosed-=Closed;}
}
