param([Parameter(Mandatory)][int]$OwnedProcessId,[Parameter(Mandatory)][string]$Executable,[Parameter(Mandatory)][string]$Pipe,[Parameter(Mandatory)][string]$Profile,[Parameter(Mandatory)][string]$OutputDirectory)
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
$process=Get-Process -Id $OwnedProcessId
if(-not [string]::Equals($process.Path,[IO.Path]::GetFullPath($Executable),[StringComparison]::OrdinalIgnoreCase)){throw 'The window does not belong to this test executable'}
& (Join-Path $PSScriptRoot '../../../../build/assert-temporary-path.ps1') -Path $OutputDirectory
Add-Type -AssemblyName System.Drawing
# Capture the actual native surface, not a CDP screenshot that can force a new
# compositor frame. Only the runner-owned process and its re-open launcher move.
Add-Type -ReferencedAssemblies System.Drawing -TypeDefinition @'
using System;
using System.Drawing;
using System.Drawing.Imaging;
using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using System.Collections.Generic;
public static class NativeResumeCheck {
 delegate bool EnumWindow(IntPtr h,IntPtr data);
 [DllImport("user32.dll")] static extern bool EnumWindows(EnumWindow callback,IntPtr data);
 [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h,out uint pid);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern int GetClassName(IntPtr h,StringBuilder name,int count);
 [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr h);
 [DllImport("user32.dll")] static extern bool IsIconic(IntPtr h);
 [DllImport("user32.dll")] static extern bool PostMessage(IntPtr h,uint message,UIntPtr w,IntPtr l);
 [DllImport("user32.dll")] static extern bool PrintWindow(IntPtr h,IntPtr dc,uint flags);
 [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr h,out Rect rect);
 [DllImport("user32.dll")] static extern bool SetProcessDPIAware();
 [StructLayout(LayoutKind.Sequential)] struct Rect {public int left,top,right,bottom;}
 public sealed class Sample { public string mode; public int iteration; public double firstFrameMs,contentReadyMs,matchingPixels; }
 static IntPtr Find(int pid){IntPtr found=IntPtr.Zero;EnumWindows(delegate(IntPtr h,IntPtr ignored){uint owner;GetWindowThreadProcessId(h,out owner);var name=new StringBuilder(128);GetClassName(h,name,128);if(owner==pid&&name.ToString()=="WinOne.Desktop")found=h;return true;},IntPtr.Zero);return found;}
 static Bitmap Shot(IntPtr h){Rect r;if(!GetWindowRect(h,out r))throw new Exception("Window bounds unavailable");var image=new Bitmap(r.right-r.left,r.bottom-r.top,PixelFormat.Format32bppArgb);using(var g=Graphics.FromImage(image)){var dc=g.GetHdc();bool ok;try{ok=PrintWindow(h,dc,2);}finally{g.ReleaseHdc(dc);}if(!ok){image.Dispose();throw new Exception("Native window capture failed");}}return image;}
 static double Match(Bitmap expected,Bitmap actual){if(expected.Size!=actual.Size)return 0;int same=0,total=0;for(int y=24;y<expected.Height-24;y+=5)for(int x=24;x<expected.Width-24;x+=5){var a=expected.GetPixel(x,y);var b=actual.GetPixel(x,y);if(Math.Abs(a.R-b.R)+Math.Abs(a.G-b.G)+Math.Abs(a.B-b.B)<36)same++;total++;}return (double)same/total;}
 static void Wait(Func<bool> test,string message){var clock=Stopwatch.StartNew();while(!test()){if(clock.ElapsedMilliseconds>5000)throw new Exception(message);Thread.Sleep(10);}}
 public static List<Sample> Run(int pid,string exe,string pipe,string profile,string output){
  SetProcessDPIAware();var h=Find(pid);if(h==IntPtr.Zero)throw new Exception("Owned settings window missing");
  Directory.CreateDirectory(output);var results=new List<Sample>();
  using(var baseline=Shot(h)){
   int low=255,high=0;for(int y=24;y<baseline.Height-24;y+=5)for(int x=24;x<baseline.Width-24;x+=5){var p=baseline.GetPixel(x,y);low=Math.Min(low,p.R);high=Math.Max(high,p.R);}if(high-low<80)throw new Exception("Baseline native surface is blank; do not count a CDP-only page as visible UI");
   baseline.Save(Path.Combine(output,"baseline.png"),ImageFormat.Png);
   foreach(var mode in new[]{"close","minimize"})for(int iteration=0;iteration<3;iteration++){
    if(!PostMessage(h,mode=="close"?0x10u:0x112u,mode=="close"?UIntPtr.Zero:new UIntPtr(0xf020u),IntPtr.Zero))throw new Exception("Could not send the owned window command");
    Wait(()=>mode=="close"?!IsWindowVisible(h):IsIconic(h),"Window did not hide/minimize");
    Thread.Sleep(iteration==2?1200:200); // Test both immediate and idle re-open, not a product delay.
    var clock=Stopwatch.StartNew();var launch=new ProcessStartInfo(exe,"--pipe \""+pipe+"\" --data-dir \""+profile+"\" --standard-session"){UseShellExecute=false,CreateNoWindow=true};
    using(var child=Process.Start(launch)){
     bool first=true;double firstMs=0,ratio=0;bool painted=false;
     while(clock.ElapsedMilliseconds<1000){
      if(IsWindowVisible(h)&&!IsIconic(h))using(var image=Shot(h)){
       if(first){first=false;firstMs=clock.Elapsed.TotalMilliseconds;image.Save(Path.Combine(output,mode+"-"+iteration+"-first.png"),ImageFormat.Png);}
       ratio=Match(baseline,image);
       if(ratio>=0.94){results.Add(new Sample{mode=mode,iteration=iteration,firstFrameMs=firstMs,contentReadyMs=clock.Elapsed.TotalMilliseconds,matchingPixels=ratio});painted=true;break;}
      }
      Thread.Sleep(10);
     }
     if(!child.WaitForExit(5000)||child.ExitCode!=0)throw new Exception("Re-open launcher did not finish successfully");
     if(!painted)throw new Exception("Native "+mode+" re-open remained blank/different for 1000 ms; match="+ratio);
     if(Find(pid)!=h)throw new Exception("Re-open replaced the existing window");
    }
   }
  }
  return results;
 }
}
'@
[NativeResumeCheck]::Run($OwnedProcessId,[IO.Path]::GetFullPath($Executable),$Pipe,$Profile,$OutputDirectory)|ConvertTo-Json -Depth 4
