using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.Linq;
using System.Windows.Forms;
using Stroke;

namespace WinOne.StrokeRuntime
{
    // Only enabled paths are candidates. Rejection restores the original down
    // and movement: a click at the end cannot restore a native right drag.
    public sealed class GestureEngine
    {
        private const int Step=15, HoldMilliseconds=250;
        private readonly Configuration config;
        private readonly ScopePolicy policy;
        private readonly Func<Point,WindowTarget> targetAt;
        private readonly System.Action<Point,Point> draw;
        private readonly System.Action clear;
        private readonly System.Action<string,int,WindowTarget> execute;
        private readonly System.Action<IList<MouseHook.MouseActionArgs>> replay;
        private readonly Func<long> now;
        private readonly Func<bool> modified;
        private readonly Dictionary<string,int[]> paths;
        private readonly List<MouseHook.MouseActionArgs> pending=new List<MouseHook.MouseActionArgs>();
        private readonly List<int> directions=new List<int>();
        private readonly HashSet<MouseButtons> consumed=new HashSet<MouseButtons>();
        private List<PackageDefinition> packages;
        private Dictionary<string,int[]> candidates;
        private WindowTarget target;
        private Point anchor,last;
        private long began;
        private bool actionTaken;
        private volatile bool tracking;
        public bool Tracking {get{return tracking;}}
        public string Diagnostic {get{return policy.Diagnostic;}}

        public GestureEngine(Configuration value,Func<Point,WindowTarget> targetAt,System.Action<Point,Point> draw,System.Action clear,System.Action<string,int,WindowTarget> execute,System.Action<IList<MouseHook.MouseActionArgs>> replay,ScopePolicy policy=null,Func<long> now=null,Func<bool> modified=null)
        {
            config=value;this.policy=policy??new ScopePolicy(value);this.targetAt=targetAt;this.draw=draw;this.clear=clear;this.execute=execute;this.replay=replay;
            this.now=now??(()=>Stopwatch.GetTimestamp()*1000/Stopwatch.Frequency);this.modified=modified??(()=>Control.ModifierKeys!=Keys.None);
            paths=value.gestures.Select(g=>Configuration.ToGesture(g)).Where(g=>g.Vectors!=null).ToDictionary(g=>g.Name,g=>Path(g));
        }
        // Quantize recorded vectors, preserving custom multi-part paths and
        // old settings. Tiny sampling noise is not another direction.
        private static int[] Path(Gesture gesture)
        {
            var result=new List<int>();int previous=-1,count=0;
            foreach(var v in gesture.Vectors){int direction=Direction(v.X,v.Y);if(direction==previous){count++;continue;}if(count>=3&&(result.Count==0||result.Last()!=previous))result.Add(previous);previous=direction;count=1;}
            if(count>=3&&(result.Count==0||result.Last()!=previous))result.Add(previous);return result.ToArray();
        }
        private static int Direction(long x,long y){return ((int)Math.Floor(Math.Atan2(y,x)*4/Math.PI+.5)+8)%8;}
        public void Tick(){if(tracking&&!actionTaken&&directions.Count==0&&now()-began>=HoldMilliseconds)ReturnNative();}
        public bool Process(MouseHook.MouseActionArgs e)
        {
            // A special button may be released after the trigger. Its matching
            // up is still consumed, while unrelated releases pass through.
            if(e.MouseButtonState==MouseHook.MouseButtonStates.Up&&consumed.Remove(e.MouseButton))return true;
            if(!tracking){
                if(e.MouseButton!=config.Button||e.MouseButtonState!=MouseHook.MouseButtonStates.Down||modified())return false;
                var next=targetAt(e.Location);var allowed=policy.Match(next);
                var enabled=new HashSet<string>(allowed.SelectMany(p=>p.actions).Where(a=>a.enabled).Select(a=>a.gesture));
                if(!enabled.Any(id=>id.StartsWith("#")||paths.ContainsKey(id)&&paths[id].Length>0))return false;
                // Copy identity; crossing a window must not change the target.
                target=new WindowTarget{window=next.window,processId=next.processId,processPath=next.processPath,windowClass=next.windowClass,point=new PointValue{x=e.Location.X,y=e.Location.Y}};
                packages=allowed;candidates=paths.Where(p=>enabled.Contains(p.Key)&&p.Value.Length>0).ToDictionary(p=>p.Key,p=>p.Value);
                anchor=last=e.Location;began=now();directions.Clear();pending.Clear();pending.Add(e);actionTaken=false;tracking=true;return true;
            }
            if(!actionTaken)pending.Add(e);
            if(!actionTaken&&directions.Count==0&&now()-began>=HoldMilliseconds){ReturnNative();return true;}
            if(e.MouseButton==config.Button&&e.MouseButtonState==MouseHook.MouseButtonStates.Up){
                if(!actionTaken){
                    string match=candidates.FirstOrDefault(p=>p.Value.SequenceEqual(directions)).Key;
                    if(match==null){ReturnNative();return true;}Run(match);
                }
                tracking=false;pending.Clear();clear();return true;
            }
            if(e.MouseButtonState==MouseHook.MouseButtonStates.Move){
                if(actionTaken)return false;
                long dx=(long)e.Location.X-anchor.X,dy=(long)e.Location.Y-anchor.Y;
                if(dx*dx+dy*dy>=Step*Step){
                    int direction=Direction(dx,dy);anchor=e.Location;
                    if(directions.Count==0||directions.Last()!=direction)directions.Add(direction);
                    foreach(string id in candidates.Where(p=>p.Value.Length<directions.Count||!p.Value.Take(directions.Count).SequenceEqual(directions)).Select(p=>p.Key).ToArray())candidates.Remove(id);
                    if(candidates.Count==0){ReturnNative();return true;}
                }
                if(pending.Count>=4096){ReturnNative();return true;}
                if(directions.Count>0)draw(last,e.Location);last=e.Location;return false;
            }
            string special=e.MouseButtonState==MouseHook.MouseButtonStates.Wheel?(e.WheelDelta>0?"#5":"#6"):Special(e.MouseButton);
            if(directions.Count==0&&special!=null&&Find(special)!=null&&(e.MouseButtonState==MouseHook.MouseButtonStates.Wheel||e.MouseButtonState==MouseHook.MouseButtonStates.Down)){
                if(e.MouseButtonState==MouseHook.MouseButtonStates.Down)consumed.Add(e.MouseButton);
                Run(special);actionTaken=true;pending.Clear();clear();return true;
            }
            // No implicit key marks: an unassigned wheel or second button is
            // delivered together with the deferred trigger, in original order.
            if(!actionTaken){ReturnNative();return true;}return false;
        }
        private void ReturnNative(){var events=pending.ToArray();tracking=false;pending.Clear();directions.Clear();clear();if(events.Length>0)replay(events);}
        private string Find(string gesture){if(packages==null)return null;for(int i=packages.Count-1;i>=0;i--){var action=packages[i].actions.FirstOrDefault(a=>a.enabled&&a.gesture==gesture);if(action!=null)return packages[i].id+"."+action.id;}return null;}
        private void Run(string gesture){string action=Find(gesture);if(action!=null)execute(action,0,target);}
        private static string Special(MouseButtons b){switch(b){case MouseButtons.Middle:return "#0";case MouseButtons.Left:return "#1";case MouseButtons.Right:return "#2";case MouseButtons.XButton1:return "#3";case MouseButtons.XButton2:return "#4";default:return null;}}
        public void Cancel(){if(tracking&&!actionTaken)ReturnNative();tracking=false;pending.Clear();packages=null;consumed.Clear();clear();}
    }
}
