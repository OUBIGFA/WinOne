using System;
using System.ComponentModel;
using System.Collections.Generic;
using System.Drawing;
using System.Runtime.InteropServices;
using System.Text;
using System.Windows.Forms;
using Stroke;

namespace WinOne.StrokeRuntime
{
    internal static class NativeWindowIdentity
    {
        [DllImport("user32.dll",CharSet=CharSet.Unicode)]private static extern int GetClassName(IntPtr window,StringBuilder name,int length);
        internal static WindowTarget At(Point point){return FromWindow(API.GetAncestor(API.WindowFromPoint(new API.POINT(point.X,point.Y)),API.GA.ROOT));}
        internal static WindowTarget FromWindow(IntPtr window)
        {
            if(window==IntPtr.Zero)return null;
            uint pid;API.GetWindowThreadProcessId(window,out pid);if(pid==0)return null;
            IntPtr process=API.OpenProcess(API.AccessRights.PROCESS_QUERY_LIMITED_INFORMATION,false,pid);if(process==IntPtr.Zero)return null;
            try{var path=new StringBuilder(32768);uint length=(uint)path.Capacity;if(!API.QueryFullProcessImageName(process,0,path,ref length))return null;var name=new StringBuilder(256);if(GetClassName(window,name,name.Capacity)==0)return null;return new WindowTarget{window=window.ToInt64(),processId=(int)pid,processPath=path.ToString(),windowClass=name.ToString()};}
            finally{API.CloseHandle(process);}
        }
    }
    // Keep the trail in the Form's paint surface. A DC obtained while the
    // layered window is hidden cannot be reused across show/repaint cycles;
    // painting through PaintEventArgs also respects clipping and WM_PRINT.
    internal sealed class TrailWindow:Form
    {
        private readonly PenDefinition pen;
        private readonly System.Drawing.Pen brush;
        private readonly List<Tuple<Point,Point>> segments=new List<Tuple<Point,Point>>();
        internal TrailWindow(PenDefinition definition)
        {
            pen=definition;AutoScaleMode=AutoScaleMode.None;FormBorderStyle=FormBorderStyle.None;ShowInTaskbar=false;ShowIcon=false;Color key=Color.FromArgb(1,2,3);if(ColorTranslator.FromHtml(definition.color).ToArgb()==key.ToArgb())key=Color.FromArgb(2,3,4);BackColor=key;TransparencyKey=key;StartPosition=FormStartPosition.Manual;Bounds=SystemInformation.VirtualScreen;Opacity=definition.opacity;
            SetStyle(ControlStyles.UserPaint|ControlStyles.AllPaintingInWmPaint|ControlStyles.OptimizedDoubleBuffer,true);
            brush=new System.Drawing.Pen(ColorTranslator.FromHtml(definition.color),Math.Max(1,definition.thickness));
        }
        protected override CreateParams CreateParams {get{var value=base.CreateParams;value.ExStyle|=(int)(API.WS_EX.TRANSPARENT|API.WS_EX.LAYERED|API.WS_EX.NOACTIVATE);return value;}}
        protected override bool ShowWithoutActivation {get{return true;}}
        internal void Segment(Point from,Point to){if(!pen.enabled||pen.opacity==0||pen.thickness==0)return;segments.Add(Tuple.Create(from,to));if(!Visible){Bounds=SystemInformation.VirtualScreen;Show();API.SetWindowPos(Handle,API.IA.TOPMOST,0,0,0,0,API.SWP.NOSIZE|API.SWP.NOMOVE|API.SWP.NOACTIVATE);}var dirty=Rectangle.FromLTRB(Math.Min(from.X,to.X)-Left,Math.Min(from.Y,to.Y)-Top,Math.Max(from.X,to.X)-Left+1,Math.Max(from.Y,to.Y)-Top+1);dirty.Inflate(pen.thickness,pen.thickness);Invalidate(dirty);Update();}
        protected override void OnPaint(PaintEventArgs e){base.OnPaint(e);foreach(var segment in segments)e.Graphics.DrawLine(brush,segment.Item1.X-Left,segment.Item1.Y-Top,segment.Item2.X-Left,segment.Item2.Y-Top);}
        internal void ClearTrail(){if(IsDisposed)return;segments.Clear();Hide();Invalidate();}
        protected override void Dispose(bool disposing){if(disposing)brush.Dispose();base.Dispose(disposing);}
    }
}
