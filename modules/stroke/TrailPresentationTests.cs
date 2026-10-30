using System;
using System.Diagnostics;
using System.Drawing;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;
using System.Runtime.InteropServices;
using API=Stroke.API;

namespace WinOne.StrokeRuntime
{
    // Opt-in compositor check: no hook, SendInput, scripts or real configuration.
    // Sampling is limited to one pixel inside this test's own white window.
    internal sealed class ScreenPixelProbe:IDisposable
    {
        [DllImport("gdi32.dll",SetLastError=true)]private static extern bool BitBlt(IntPtr target,int x,int y,int width,int height,IntPtr source,int sourceX,int sourceY,uint operation);
        [DllImport("gdi32.dll")]private static extern uint GetPixel(IntPtr canvas,int x,int y);
        private readonly Bitmap bitmap=new Bitmap(1,1);
        private readonly Graphics graphics;
        private readonly IntPtr canvas,screen;
        internal ScreenPixelProbe(){graphics=Graphics.FromImage(bitmap);canvas=graphics.GetHdc();screen=API.GetDC(IntPtr.Zero);}
        internal Color Read(int x,int y){
            // .NET Framework CopyFromScreen rejects combined enum flags.
            // CAPTUREBLT is required to include a layered trail in this probe.
            if(screen==IntPtr.Zero||!BitBlt(canvas,0,0,1,1,screen,x,y,0x40cc0020))throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
            uint color=GetPixel(canvas,0,0);if(color==UInt32.MaxValue)throw new Exception("Fixture pixel unavailable");
            return Color.FromArgb((int)(color&255),(int)((color>>8)&255),(int)((color>>16)&255));
        }
        public void Dispose(){if(screen!=IntPtr.Zero)API.ReleaseDC(IntPtr.Zero,screen);graphics.ReleaseHdc(canvas);graphics.Dispose();bitmap.Dispose();}
    }

    internal static class TrailPresentationTests
    {
        internal static void Run()
        {
            using(var form=new Form{Text="WinOne trail presentation fixture",BackColor=Color.White,TopMost=true,StartPosition=FormStartPosition.CenterScreen,Size=new Size(420,300)})
            using(var trail=new TrailWindow(new PenDefinition{enabled=true,color="#1f7fff",opacity=.8,thickness=5}))
            {
                form.Show();form.Activate();Pump(150);
                var from=form.PointToScreen(new Point(180,100));var to=form.PointToScreen(new Point(60,100));
                var hit=NativeWindowIdentity.At(to);
                if(hit==null||hit.window!=form.Handle.ToInt64())throw new Exception("Trail fixture is obscured; compositor check not run");
                // Positive control distinguishes a presentation failure from a
                // capture environment that cannot observe layered windows.
                trail.Segment(from,to);Pump(100);
                using(var pixel=new ScreenPixelProbe()){
                    var color=pixel.Read(to.X+40,to.Y);
                    if(color.B<=color.R+60)throw new Exception("Held trail is not visibly composited: "+color+"; visible="+trail.Visible+"; bounds="+trail.Bounds+"; sample="+to);
                }
                trail.ClearTrail();Pump(50);
                int blue=0;bool finished=false;
                using(var ready=new ManualResetEventSlim(false))
                using(var stop=new CancellationTokenSource())
                {
                    var observer=Task.Run(()=>{
                        using(var pixel=new ScreenPixelProbe()){
                            ready.Set();
                            while(!stop.IsCancellationRequested){
                                var color=pixel.Read(to.X+40,to.Y);if(color.B>color.R+60)Interlocked.Increment(ref blue);
                                Thread.Sleep(1);
                            }
                        }
                    });
                    ready.Wait();
                    // The worker can finish a quick stroke before the UI drains
                    // its queue. Update() paints the DC but is not presentation.
                    form.BeginInvoke(new Action(()=>trail.Segment(from,to)));
                    form.BeginInvoke(new Action(()=>{trail.ClearTrail();finished=true;}));
                    Pump(150);stop.Cancel();observer.GetAwaiter().GetResult();
                    if(!finished||trail.Visible)throw new Exception("Queued trail did not finish and release its window");
                    if(blue==0)throw new Exception("Queued gesture painted and hid without a single visible compositor frame");
                    Console.WriteLine("PASS: queued short gesture visibly composited before clear (blue samples="+blue+")");
                }
            }
        }
        private static void Pump(int milliseconds){var watch=Stopwatch.StartNew();while(watch.ElapsedMilliseconds<milliseconds){Application.DoEvents();Thread.Sleep(1);}}
    }
}
