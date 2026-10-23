using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Drawing;
using System.Runtime.InteropServices;
using System.Windows.Forms;
using Stroke;

namespace WinOne.StrokeRuntime
{
    internal static class NativeGestureInput
    {
        [DllImport("user32.dll",EntryPoint="SendInput",SetLastError=true)] private static extern uint Send(uint count,[In] API.INPUT[] inputs,int size);
        internal static API.INPUT[] Encode(IList<MouseHook.MouseActionArgs> events,Rectangle screen)
        {
            var inputs=new List<API.INPUT>();
            foreach(var e in events){
                // Reposition before each original event, including the down at
                // the source item. One SendInput batch keeps replay ordered.
                var move=new API.INPUT{type=API.INPUTTYPE.MOUSE};
                move.mi.dx=(int)Math.Round((e.Location.X-screen.Left)*65535.0/Math.Max(1,screen.Width-1));
                move.mi.dy=(int)Math.Round((e.Location.Y-screen.Top)*65535.0/Math.Max(1,screen.Height-1));
                move.mi.dwFlags=(API.MOUSEEVENTF)0xc001;move.mi.dwExtraInfo=(UIntPtr)0x7fu;inputs.Add(move);
                if(e.MouseButtonState==MouseHook.MouseButtonStates.Move)continue;
                var input=new API.INPUT{type=API.INPUTTYPE.MOUSE};input.mi.dwExtraInfo=(UIntPtr)0x7fu;
                if(e.MouseButtonState==MouseHook.MouseButtonStates.Wheel){input.mi.dwFlags=(API.MOUSEEVENTF)0x800;input.mi.mouseData=unchecked((uint)(int)e.WheelDelta);}
                else {
                    uint flag;switch(e.MouseButton){case MouseButtons.Left:flag=2;break;case MouseButtons.Right:flag=8;break;case MouseButtons.Middle:flag=0x20;break;case MouseButtons.XButton1:flag=0x80;input.mi.mouseData=1;break;case MouseButtons.XButton2:flag=0x80;input.mi.mouseData=2;break;default:continue;}
                    if(e.MouseButtonState==MouseHook.MouseButtonStates.Up)flag*=2;input.mi.dwFlags=(API.MOUSEEVENTF)flag;
                }
                inputs.Add(input);
            }
            return inputs.ToArray();
        }
        internal static void Replay(IList<MouseHook.MouseActionArgs> events)
        {
            var inputs=Encode(events,SystemInformation.VirtualScreen);
            if(inputs.Length>0&&Send((uint)inputs.Length,inputs,Marshal.SizeOf(typeof(API.INPUT)))!=inputs.Length)throw new Win32Exception(Marshal.GetLastWin32Error(),"原生鼠标操作回放失败");
        }
    }
}
