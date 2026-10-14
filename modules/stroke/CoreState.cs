using System;
using System.Collections.Generic;
using System.Drawing;
using System.Runtime.InteropServices;
using System.Threading;
using System.Windows.Forms;

namespace Stroke
{
    // Preserve the upstream Base API without retaining its independent Form,
    // tray or configuration process. Each action process receives one snapshot.
    public static class Stroke
    {
        private sealed class CallContext { internal IntPtr Window;internal string Path;internal Point Point; }
        private static readonly AsyncLocal<CallContext> local=new AsyncLocal<CallContext>();
        private static CallContext latest=new CallContext();
        private static CallContext Current {get{return local.Value??Volatile.Read(ref latest);}}
        public static void SetContext(IntPtr window,string path,Point point){var context=new CallContext{Window=window,Path=path,Point=point};local.Value=context;Volatile.Write(ref latest,context);}
        public static IntPtr CurrentWindow { get{return Current.Window;} set{SetContext(value,Current.Path,Current.Point);} }
        public static string CurrentProcessImagePath { get{return Current.Path;} set{SetContext(Current.Window,value,Current.Point);} }
        public static Point KeyPoint { get{return Current.Point;} set{SetContext(Current.Window,Current.Path,value);} }
    }
    public static class Script
    {
        public static System.Action<string,int> Dispatch;
        public static void RunScript(string name,int mark){if(Dispatch==null)throw new InvalidOperationException("Stroke script dispatcher is unavailable");Dispatch(name,mark);}
    }
    public static class Settings
    {
        public static MouseButtons StrokeButton = MouseButtons.Right;
        public static Pen Pen = new Pen(Color.FromArgb(31,127,255),0.8,5);
        public static List<Gesture> Gestures = new List<Gesture>();
        public static List<ActionPackage> ActionPackages = new List<ActionPackage>();
        public static List<string> Namespaces = new List<string>();
        public static List<string> Assemblies = new List<string>();
        public static List<string> Filtrations = new List<string>();
        public static int ScriptThreadCount = 8;
    }
    public static class InputRouting
    {
        public static Func<API.INPUT,uint> Sink;
        public static uint Send(API.INPUT input)
        {
            if (Sink != null) return Sink(input);
            uint result = API.NativeSendInput(1,ref input,Marshal.SizeOf(typeof(API.INPUT)));
            if (result != 1) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error(),"SendInput failed");
            return result;
        }
    }
}
