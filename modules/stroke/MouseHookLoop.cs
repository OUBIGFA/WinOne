using System;
using System.ComponentModel;
using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Threading;
using System.Windows.Forms;
using Stroke;

namespace WinOne.StrokeRuntime
{
    // The low-level hook has its own message pump. Painting, script startup and
    // settings work on the application's UI thread cannot stall this callback.
    internal sealed class MouseHookLoop:IDisposable
    {
        private readonly Thread thread;
        private readonly ManualResetEvent started=new ManualResetEvent(false);
        private readonly System.Action reset;
        private readonly System.Action tick;
        private HookWindow window;
        private Exception startupError;
        private bool disposed;
        private volatile string diagnostic;
        internal string Diagnostic {get{return diagnostic;}private set{diagnostic=value;}}
        internal MouseHookLoop(System.Action reset,System.Action tick)
        {
            this.reset=reset;this.tick=tick;
            thread=new Thread(Run){IsBackground=true,Name="WinOne mouse hook"};
            thread.SetApartmentState(ApartmentState.STA);thread.Start();
            started.WaitOne();started.Dispose();
            if(startupError!=null)throw new InvalidOperationException("鼠标手势监听无法启动",startupError);
        }
        private void Run()
        {
            try{
                using(var listener=new HookWindow(this)){
                    window=listener;MouseHook.StartHook();started.Set();Application.Run();
                }
            }catch(Exception error){startupError=error;Diagnostic=error.Message;Console.Error.WriteLine("stroke.hook: "+error.GetType().Name);try{started.Set();}catch(ObjectDisposedException){}}
            finally{if(MouseHook.Enable)try{MouseHook.StopHook();}catch(Exception error){Console.Error.WriteLine("stroke.hook.stop: "+error.GetType().Name);}}
        }
        private void Recover()
        {
            try{
                MouseHook.StopHook();reset();MouseHook.StartHook();Diagnostic=null;
                Console.Error.WriteLine("stroke.hook: restored after missing low-level callbacks");
            }catch(Exception error){Diagnostic="鼠标手势监听恢复失败："+error.Message;Console.Error.WriteLine("stroke.hook.recover: "+error.GetType().Name);}
        }
        public void Dispose()
        {
            if(disposed)return;disposed=true;
            var listener=window;
            if(listener!=null&&!listener.IsDisposed)try{listener.BeginInvoke(new System.Action(Application.ExitThread));}catch(InvalidOperationException){}
            if(!thread.Join(3000))throw new InvalidOperationException("鼠标手势监听线程未退出");
        }
        private sealed class HookWindow:Control
        {
            [StructLayout(LayoutKind.Sequential)]private struct RawDevice {public ushort page,usage;public uint flags;public IntPtr target;}
            [DllImport("user32.dll",SetLastError=true)]private static extern bool RegisterRawInputDevices([In] RawDevice[] devices,uint count,uint size);
            private readonly System.Windows.Forms.Timer timer=new System.Windows.Forms.Timer{Interval=1000};
            private readonly System.Windows.Forms.Timer pendingTimer=new System.Windows.Forms.Timer{Interval=25};
            private readonly HookRecovery health=new HookRecovery();
            private long inputAt;
            private static long Milliseconds(long ticks){return ticks/(Stopwatch.Frequency/1000);}
            internal HookWindow(MouseHookLoop owner)
            {
                // RIDEV_INPUTSINK observes mouse activity without intercepting
                // input or injecting a synthetic click, key or mouse movement.
                if(!RegisterRawInputDevices(new[]{new RawDevice{page=1,usage=2,flags=0x100,target=Handle}},1,(uint)Marshal.SizeOf(typeof(RawDevice))))throw new Win32Exception(Marshal.GetLastWin32Error());
                timer.Tick+=(_,__)=>{if(health.NeedsRestart(Milliseconds(Stopwatch.GetTimestamp()),inputAt,Milliseconds(MouseHook.LastEventTimestamp)))owner.Recover();};
                timer.Start();
                pendingTimer.Tick+=(_,__)=>owner.tick();pendingTimer.Start();
            }
            protected override void WndProc(ref Message message)
            {
                if(message.Msg==0x00ff)inputAt=Milliseconds(Stopwatch.GetTimestamp());
                base.WndProc(ref message);
            }
            protected override void Dispose(bool disposing)
            {
                if(disposing){pendingTimer.Stop();pendingTimer.Dispose();timer.Stop();timer.Dispose();if(IsHandleCreated)RegisterRawInputDevices(new[]{new RawDevice{page=1,usage=2,flags=1,target=IntPtr.Zero}},1,(uint)Marshal.SizeOf(typeof(RawDevice)));}
                base.Dispose(disposing);
            }
        }
    }
}
