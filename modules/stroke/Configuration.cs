using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Text.RegularExpressions;
using System.Web.Script.Serialization;
using System.Windows.Forms;

namespace WinOne.StrokeRuntime
{
    public sealed class PointValue { public int x; public int y; public Point ToPoint(){return new Point(x,y);} }
    public sealed class GestureDefinition { public string id; public string name; public PointValue[] points; public int[][] vectors; }
    public sealed class ActionDefinition { public string id; public string name; public string gesture; public string code; public bool enabled; }
    public sealed class PackageDefinition { public string id; public string name; public ActionDefinition[] actions; }
    public sealed class ScopeDefinition { public string id; public string name; public string kind; public bool enabled; public string processPath; public string pattern; public string[] windowClasses; public string[] packages; }
    public sealed class PenDefinition { public bool enabled=true; public string color; public double opacity; public int thickness; }
    public sealed class Configuration
    {
        public int version; public ulong revision; public string defaultPolicy; public string triggerButton;
        public PenDefinition pen; public GestureDefinition[] gestures; public PackageDefinition[] actionPackages;
        public ScopeDefinition[] scopes; public string[] filters; public int scriptThreadCount; public int scriptTimeoutSeconds;
        public string[] namespaces; public string[] assemblies;
        public static readonly string[] AllowedAssemblies={"System.dll","System.Core.dll","System.Drawing.dll","System.Windows.Forms.dll","System.Xml.dll","System.Xml.Linq.dll","System.Net.Http.dll","System.Web.dll","Microsoft.CSharp.dll"};
        public MouseButtons Button { get {switch(triggerButton){case "right":return MouseButtons.Right;case "left":return MouseButtons.Left;case "middle":return MouseButtons.Middle;case "x1":return MouseButtons.XButton1;case "x2":return MouseButtons.XButton2;default:throw new InvalidDataException("Invalid trigger button");}} }
        public static Configuration Parse(string text)
        {
            var json=new JavaScriptSerializer{MaxJsonLength=1024*1024,RecursionLimit=48};
            Configuration value=json.Deserialize<Configuration>(text);if(value==null)throw new InvalidDataException("Settings are missing");value.Validate();return value;
        }
        public void Validate()
        {
            if(version!=1||defaultPolicy!="disabled"||pen==null||gestures==null||actionPackages==null||scopes==null||filters==null||namespaces==null||assemblies==null)throw new InvalidDataException("Unsupported Stroke settings");
            var button=Button;
            if(!Regex.IsMatch(pen.color??"","^#[0-9a-fA-F]{6}$")||pen.opacity<0||pen.opacity>1||pen.thickness<0||pen.thickness>10)throw new InvalidDataException("Invalid pen");
            if(scriptThreadCount<1||scriptThreadCount>64||scriptTimeoutSeconds<0||scriptTimeoutSeconds>3600)throw new InvalidDataException("Invalid action limits");
            if(gestures.Length>256||actionPackages.Length>32||scopes.Length>64||filters.Length>32)throw new InvalidDataException("Too many settings");
            var ids=new HashSet<string>();
            foreach(var gesture in gestures){if(!Id(gesture.id)||!ids.Add(gesture.id)||!Name(gesture.name))throw new InvalidDataException("Invalid gesture identity");ToGesture(gesture);}
            var packs=new HashSet<string>();
            foreach(var pack in actionPackages){if(!Id(pack.id)||!packs.Add(pack.id)||!Name(pack.name)||pack.actions==null||pack.actions.Length>256)throw new InvalidDataException("Invalid action package");var names=new HashSet<string>();foreach(var action in pack.actions){if(!Id(action.id)||!names.Add(action.id)||!Name(action.name)||action.code==null||action.code.Length>32000||(!ids.Contains(action.gesture)&&!Regex.IsMatch(action.gesture??"","^#[0-6]$")))throw new InvalidDataException("Invalid action");}}
            var scopeIds=new HashSet<string>();
            foreach(var scope in scopes){if(!Id(scope.id)||!scopeIds.Add(scope.id)||!Name(scope.name)||!new[]{"explorer-folders","application","global"}.Contains(scope.kind)||scope.packages==null||scope.packages.Any(id=>!packs.Contains(id)))throw new InvalidDataException("Invalid scope");if(scope.kind=="application"&&String.IsNullOrWhiteSpace(scope.processPath)&&String.IsNullOrWhiteSpace(scope.pattern))throw new InvalidDataException("Application scope needs a path or pattern");if(!String.IsNullOrEmpty(scope.pattern)){if(scope.pattern.Length>8192)throw new InvalidDataException("Scope patterns too long");foreach(string pattern in scope.pattern.Split(new[]{'\r','\n'},StringSplitOptions.RemoveEmptyEntries))CompilePattern(pattern);}}
            foreach(string filter in filters)CompilePattern(filter);
            if(namespaces.Length>128||assemblies.Length>64||namespaces.Any(n=>String.IsNullOrWhiteSpace(n)||n.Length>512||n.IndexOfAny(new[]{'\r','\n','\0'})>=0)||assemblies.Any(a=>!Regex.IsMatch(a??"",@"(?i)^[A-Za-z0-9_.-]+[.]dll$")&&(!Regex.IsMatch(a??"",@"(?i)^[A-Za-z]:[\\/].+[.]dll$")||a.IndexOfAny(new[]{'\r','\n','\0'})>=0)))throw new InvalidDataException("Invalid script references");
        }
        private static bool Id(string value){return Regex.IsMatch(value??"","^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$");}
        private static bool Name(string value){return !String.IsNullOrWhiteSpace(value)&&value.Length<=240&&value.IndexOfAny(new[]{'\r','\n','\0'})<0;}
        internal static Regex CompilePattern(string pattern){if(pattern==null||pattern.Length>512)throw new InvalidDataException("Pattern too long");return new Regex(pattern,RegexOptions.CultureInvariant,TimeSpan.FromMilliseconds(2));}
        public static global::Stroke.Gesture ToGesture(GestureDefinition definition)
        {
            var gesture=new global::Stroke.Gesture(definition.id);
            if(definition.vectors!=null&&definition.vectors.Length>0){if(definition.vectors.Length!=128)throw new InvalidDataException("Stroke needs 128 vectors");gesture.Vectors=definition.vectors.Select(v=>{if(v==null||v.Length!=2||v.Any(n=>n< -127||n>127)||v[0]==0&&v[1]==0)throw new InvalidDataException("Invalid gesture vector");return new global::Stroke.Gesture.Vector((sbyte)v[0],(sbyte)v[1]);}).ToArray();}
            else {if(definition.points==null||definition.points.Length==0)return gesture;if(definition.points.Length<2||definition.points.Length>4096||definition.points.Any(p=>p==null||Math.Abs((long)p.x)>100000||Math.Abs((long)p.y)>100000))throw new InvalidDataException("Invalid gesture path");gesture.GenerateVectors(definition.points.Select(p=>p.ToPoint()).ToList());}
            return gesture;
        }
        public void ApplyBase()
        {
            global::Stroke.Settings.StrokeButton=Button;
            global::Stroke.Settings.Pen=new global::Stroke.Pen(ColorTranslator.FromHtml(pen.color),pen.opacity,(byte)pen.thickness);
            global::Stroke.Settings.Gestures=gestures.Select(g=>{var value=ToGesture(g);value.Name=g.name;return value;}).ToList();
            global::Stroke.Settings.Namespaces=namespaces.ToList();global::Stroke.Settings.Assemblies=assemblies.ToList();
            global::Stroke.Settings.Filtrations=filters.ToList();global::Stroke.Settings.ScriptThreadCount=scriptThreadCount;
            global::Stroke.Settings.ActionPackages=actionPackages.Select(p=>{var pack=new global::Stroke.ActionPackage(p.name,String.Join("\n",scopes.Where(s=>s.packages.Contains(p.id)).Select(s=>s.pattern??"")));pack.Actions=p.actions.Select(a=>new global::Stroke.Action(a.name,gestures.FirstOrDefault(g=>g.id==a.gesture)?.name??a.gesture,a.code)).ToList();return pack;}).ToList();
        }
    }
}
