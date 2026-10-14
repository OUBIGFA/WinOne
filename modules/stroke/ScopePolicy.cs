using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Text.RegularExpressions;

namespace WinOne.StrokeRuntime
{
    public sealed class WindowTarget { public long window; public int processId; public string processPath; public string windowClass; public PointValue point; }
    public sealed class ScopePolicy
    {
        private readonly Configuration config;
        private readonly Dictionary<string,Regex[]> patterns=new Dictionary<string,Regex[]>();
        private readonly Regex[] filters;
        private readonly string explorer;
        public string Diagnostic {get;private set;}
        public ScopePolicy(Configuration value,string windowsDirectory=null)
        {
            config=value;explorer=Path.Combine(windowsDirectory??Environment.GetFolderPath(Environment.SpecialFolder.Windows),"explorer.exe");
            foreach(var scope in value.scopes)if(!String.IsNullOrEmpty(scope.pattern))patterns[scope.id]=scope.pattern.Split(new[]{'\r','\n'},StringSplitOptions.RemoveEmptyEntries).Select(Configuration.CompilePattern).ToArray();
            filters=value.filters.Select(Configuration.CompilePattern).ToArray();
        }
        // Match a verified window/process snapshot before consuming the trigger.
        // explorer.exe also owns the desktop/taskbar; process name alone is unsafe.
        public List<PackageDefinition> Match(WindowTarget target)
        {
            Diagnostic=null;
            var result=new List<PackageDefinition>();
            if(target==null||target.window==0||target.processId<=0||String.IsNullOrEmpty(target.processPath)||String.IsNullOrEmpty(target.windowClass))return result;
            try {
                if(filters.Any(filter=>filter.IsMatch(target.processPath)))return result;
                foreach(var scope in config.scopes){
                    if(!scope.enabled)continue;
                    bool match=scope.kind=="explorer-folders"?String.Equals(target.processPath,explorer,StringComparison.OrdinalIgnoreCase)&&(target.windowClass=="CabinetWClass"||target.windowClass=="ExploreWClass"):
                        scope.kind=="global"||(!String.IsNullOrWhiteSpace(scope.processPath)?String.Equals(target.processPath,Environment.ExpandEnvironmentVariables(scope.processPath),StringComparison.OrdinalIgnoreCase):patterns[scope.id].Any(pattern=>pattern.IsMatch(target.processPath)));
                    if(!match||scope.windowClasses!=null&&scope.windowClasses.Length>0&&!scope.windowClasses.Contains(target.windowClass))continue;
                    foreach(string id in scope.packages){var pack=config.actionPackages.First(p=>p.id==id);if(!result.Contains(pack))result.Add(pack);}
                }
            }catch(RegexMatchTimeoutException){result.Clear();Diagnostic="应用规则匹配超时；此次鼠标输入已放行";}
            return config.actionPackages.Where(pack=>result.Contains(pack)).ToList();
        }
    }
}
