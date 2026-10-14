using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using Stroke;

namespace WinOne.StrokeRuntime
{
    public sealed class InputValue
    {
        public int type; public uint flags; public int dx; public int dy; public uint data; public ushort key; public ushort scan;
        public static InputValue From(API.INPUT input){return input.type==API.INPUTTYPE.KEYBOARD?new InputValue{type=1,flags=(uint)input.ki.dwFlags,key=(ushort)input.ki.wVk,scan=input.ki.wScan}:new InputValue{type=0,flags=(uint)input.mi.dwFlags,dx=input.mi.dx,dy=input.mi.dy,data=input.mi.mouseData};}
        public API.INPUT ToInput(){var input=new API.INPUT();input.type=(API.INPUTTYPE)type;if(type==1){input.ki.wVk=(API.VK)key;input.ki.wScan=scan;input.ki.dwFlags=(API.KEYEVENTF)flags;input.ki.dwExtraInfo=(UIntPtr)0x7F;}else if(type==0){input.mi.dx=dx;input.mi.dy=dy;input.mi.mouseData=data;input.mi.dwFlags=(API.MOUSEEVENTF)flags;input.mi.dwExtraInfo=(UIntPtr)0x7F;}else throw new ArgumentException("Unsupported input type");return input;}
    }
    // Only inputs issued by this process are returned. Physically held keys at
    // the start of a script are preserved rather than released by its KeyUp.
    internal sealed class InputLedger
    {
        private readonly object gate=new object();
        private readonly Dictionary<string,Dictionary<string,InputValue>> held=new Dictionary<string,Dictionary<string,InputValue>>();
        private readonly Dictionary<string,HashSet<string>> preserved=new Dictionary<string,HashSet<string>>();
        private readonly Func<API.INPUT,uint> send;
        private readonly Func<int,bool> physical;
        [DllImport("user32.dll")]private static extern short GetAsyncKeyState(int key);
        internal InputLedger(Func<API.INPUT,uint> send=null,Func<int,bool> physical=null){this.send=send??Native;this.physical=physical??(key=>(GetAsyncKeyState(key)&0x8000)!=0);}
        private static uint Native(API.INPUT input){uint result=API.NativeSendInput(1,ref input,Marshal.SizeOf(typeof(API.INPUT)));if(result!=1)throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error(),"SendInput failed");return result;}
        internal uint Send(string owner,InputValue value)
        {
            lock(gate){
                Dictionary<string,InputValue> keys;if(!held.TryGetValue(owner,out keys)){keys=new Dictionary<string,InputValue>();held[owner]=keys;preserved[owner]=new HashSet<string>();}
                string id;int virtualKey;bool down;InputValue release;
                if(Transition(value,out id,out virtualKey,out down,out release)){
                    bool ours=false,wasPreserved=false;foreach(var entry in held.Values)ours=ours||entry.ContainsKey(id);foreach(var entry in preserved.Values)wasPreserved=wasPreserved||entry.Contains(id);
                    if(down&&!ours&&physical(virtualKey)){preserved[owner].Add(id);return 1;}
                    if(!down&&wasPreserved&&!ours){foreach(var entry in preserved.Values)entry.Remove(id);PruneEmpty();return 1;}
                    uint sent=send(value.ToInput());
                    // KeyUp may be a later gesture. Move/remove the single key
                    // owner globally so successful actions can intentionally
                    // hold a key until another action releases it.
                    foreach(var entry in held.Values)entry.Remove(id);foreach(var entry in preserved.Values)entry.Remove(id);
                    if(down)keys[id]=release;PruneEmpty();return sent;
                }
                return send(value.ToInput());
            }
        }
        private static bool Transition(InputValue value,out string id,out int key,out bool down,out InputValue release)
        {
            id=null;key=0;down=false;release=null;
            if(value.type==1){id="key:"+value.key+":"+value.scan;key=value.key;down=(value.flags&2)==0;release=new InputValue{type=1,key=value.key,scan=value.scan,flags=value.flags|2};return true;}
            uint[] downs={2,8,32,128},ups={4,16,64,256};int[] keys={1,2,4,value.data==2?6:5};
            for(int i=0;i<downs.Length;i++)if((value.flags&(downs[i]|ups[i]))!=0){id="mouse:"+keys[i];key=keys[i];down=(value.flags&downs[i])!=0;release=new InputValue{type=0,flags=ups[i],data=value.data};return true;}return false;
        }
        internal void Release(string owner)
        {
            lock(gate){Dictionary<string,InputValue> keys;if(!held.TryGetValue(owner,out keys))return;Exception failure=null;foreach(var release in keys.Values){try{send(release.ToInput());}catch(Exception error){failure=error;}}keys.Clear();held.Remove(owner);preserved.Remove(owner);if(failure!=null)throw new InvalidOperationException("Could not return a simulated key",failure);}
        }
        internal void ReleaseAll(){lock(gate){foreach(string owner in new List<string>(held.Keys))Release(owner);}}
        internal void Retire(string owner){lock(gate)PruneEmpty();}
        private void PruneEmpty(){foreach(string owner in new List<string>(held.Keys))if(held[owner].Count==0&&preserved[owner].Count==0){held.Remove(owner);preserved.Remove(owner);}}
    }
}
