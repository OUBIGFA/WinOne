using System;
using System.Collections.Generic;
using System.Drawing;
using System.Linq;
using System.Windows.Forms;
using Stroke;

namespace WinOne.StrokeRuntime
{
    // Adapted from Stroke.cs at cc72a7a: the same movement threshold, special
    // gestures, 128-vector comparison and last-matching-package precedence.
    // Resource ownership and allow-list checks are WinOne adapter boundaries.
    public sealed class GestureEngine
    {
        private readonly Configuration config;
        private readonly ScopePolicy policy;
        private readonly List<Gesture> gestures;
        private readonly Func<Point,WindowTarget> targetAt;
        private readonly System.Action<Point,Point> draw;
        private readonly System.Action clear;
        private readonly System.Action<string,int,WindowTarget> execute;
        private readonly System.Action replayClick;
        private List<PackageDefinition> packages;
        private readonly List<Point> points=new List<Point>();
        private readonly HashSet<MouseButtons> consumed=new HashSet<MouseButtons>();
        private Point start,last;
        private WindowTarget target;
        private bool moved,actionTaken;
        private int mark;
        private volatile bool tracking;
        public bool Tracking {get{return tracking;}private set{tracking=value;}}
        public string Diagnostic {get{return policy.Diagnostic;}}
        public GestureEngine(Configuration value,Func<Point,WindowTarget> targetAt,System.Action<Point,Point> draw,System.Action clear,System.Action<string,int,WindowTarget> execute,System.Action replayClick,ScopePolicy policy=null)
        {
            config=value;this.policy=policy??new ScopePolicy(value);gestures=value.gestures.Select(Configuration.ToGesture).ToList();this.targetAt=targetAt;this.draw=draw;this.clear=clear;this.execute=execute;this.replayClick=replayClick;
        }
        public bool Process(MouseHook.MouseActionArgs e)
        {
            if(e.MouseButton==config.Button&&e.MouseButtonState==MouseHook.MouseButtonStates.Down){
                if(Tracking)return true;
                WindowTarget next=targetAt(e.Location);var allowed=policy.Match(next);
                if(!allowed.Any(p=>p.actions.Any(a=>a.enabled)))return false;
                target=next;target.point=new PointValue{x=e.Location.X,y=e.Location.Y};packages=allowed;start=last=e.Location;points.Clear();points.Add(start);consumed.Clear();mark=0;moved=actionTaken=false;Tracking=true;return true;
            }
            if(!Tracking)return false;
            if(e.MouseButton==config.Button&&e.MouseButtonState==MouseHook.MouseButtonStates.Up){
                Tracking=false;clear();
                if(!actionTaken){if(moved){var sample=new Gesture("",new List<Point>(points));int score=0;Gesture best=null;foreach(var candidate in gestures){if(candidate.Vectors==null)continue;int similarity=sample.Similarity(candidate);if(similarity>score){score=similarity;best=candidate;}}if(best!=null&&score>80)Run(best.Name);}else replayClick();}
                points.Clear();return true;
            }
            if(e.MouseButtonState==MouseHook.MouseButtonStates.Move){
                if(actionTaken)return false;
                long dx=(long)e.Location.X-start.X,dy=(long)e.Location.Y-start.Y;
                moved=moved||dx*dx+dy*dy>512;
                if(points.Count>=4096){actionTaken=true;points.Clear();clear();return false;}
                if(moved)draw(last,e.Location); // The trail callback owns visibility; recognition still receives every point when the pen is off.
                last=e.Location;points.Add(last);return false;
            }
            if(e.MouseButtonState==MouseHook.MouseButtonStates.Wheel){
                string id=e.WheelDelta>0?"#5":"#6";
                if(Find(id)!=null){Run(id);actionTaken=true;clear();}return true;
            }
            string special=Special(e.MouseButton);
            if(e.MouseButtonState==MouseHook.MouseButtonStates.Down){
                mark|=Mark(e.MouseButton);
                consumed.Add(e.MouseButton);return true;
            }
            if(e.MouseButtonState==MouseHook.MouseButtonStates.Up&&consumed.Remove(e.MouseButton)){if(!moved&&!actionTaken&&special!=null&&Find(special)!=null){Run(special);actionTaken=true;clear();}return true;}
            return false;
        }
        private string Find(string gesture)
        {
            if(packages==null)return null;
            for(int i=packages.Count-1;i>=0;i--){var action=packages[i].actions.FirstOrDefault(a=>a.enabled&&a.gesture==gesture);if(action!=null)return packages[i].id+"."+action.id;}
            return null;
        }
        private void Run(string gesture){string action=Find(gesture);if(action!=null)execute(action,mark,target);}
        private static string Special(MouseButtons b){switch(b){case MouseButtons.Middle:return "#0";case MouseButtons.Left:return "#1";case MouseButtons.Right:return "#2";case MouseButtons.XButton1:return "#3";case MouseButtons.XButton2:return "#4";default:return null;}}
        private static int Mark(MouseButtons b){switch(b){case MouseButtons.Left:return 1;case MouseButtons.Right:return 2;case MouseButtons.XButton1:return 4;case MouseButtons.XButton2:return 8;case MouseButtons.Middle:return 16;default:return 0;}}
        public void Cancel(){Tracking=false;points.Clear();packages=null;consumed.Clear();clear();}
    }
}
