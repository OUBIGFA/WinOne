using System;
using System.CodeDom.Compiler;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using Stroke;

namespace WinOne.StrokeRuntime
{
    internal sealed class ScriptPlan {internal string assembly;internal Configuration configuration;internal Dictionary<string,string> functions;}
    internal static class ScriptCompiler
    {
        private static readonly object compileGate=new object();
        private static System.Diagnostics.Process activeCompiler;
        private static bool stopRequested;
        internal static void RequestStop(){lock(compileGate)stopRequested=true;}
        internal static void CancelActive(){lock(compileGate){try{if(activeCompiler!=null&&!activeCompiler.HasExited)activeCompiler.Kill();}catch(InvalidOperationException){}}}
        // The source layout is Stroke.Script.GenerateSource. Explicit system
        // references replace upstream's scan of every DLL beside the program.
        internal static ScriptPlan Compile(Configuration config,string directory)
        {
            Directory.CreateDirectory(directory);var functions=new Dictionary<string,string>();var source=new StringBuilder();
            foreach(string name in config.namespaces)source.Append("using ").Append(name).AppendLine(";");
            source.AppendLine("namespace Stroke { public static class Scripts {");int index=0;
            foreach(var pack in config.actionPackages)foreach(var action in pack.actions){string name=pack.id+"."+action.id;string method="Function_"+(index++);functions.Add(name,method);string alias=pack.name+"."+action.name;if(!functions.ContainsKey(alias))functions.Add(alias,method);source.Append("public static void ").Append(method).AppendLine("(int _) {");source.AppendLine("#line 1 \""+pack.id+"/"+action.id+"\"");source.AppendLine(action.code);source.AppendLine("#line default\n}");}
            source.AppendLine("}}");string output=Path.Combine(directory,"actions-"+Guid.NewGuid().ToString("N")+".dll");
            string sourcePath=Path.ChangeExtension(output,".cs");File.WriteAllText(sourcePath,source.ToString(),new UTF8Encoding(false));
            var references=new List<string>{typeof(global::Stroke.Base).Assembly.Location};references.AddRange(config.assemblies);
            foreach(string assembly in references)if(Path.IsPathRooted(assembly)&&!File.Exists(assembly))throw new FileNotFoundException("程序集不存在",assembly);
            Func<string,string> quote=value=>"\""+value+"\"";
            var info=new System.Diagnostics.ProcessStartInfo(Path.Combine(System.Runtime.InteropServices.RuntimeEnvironment.GetRuntimeDirectory(),"csc.exe"),"/nologo /utf8output /codepage:65001 /target:library /optimize+ /out:"+quote(output)+" "+String.Join(" ",references.Select(path=>"/reference:"+quote(path)))+" "+quote(sourcePath)){UseShellExecute=false,CreateNoWindow=true,RedirectStandardOutput=true,RedirectStandardError=true,StandardOutputEncoding=new UTF8Encoding(false),StandardErrorEncoding=new UTF8Encoding(false),WorkingDirectory=directory};
            using(var compiler=new System.Diagnostics.Process{StartInfo=info}){try{lock(compileGate){if(stopRequested)throw new OperationCanceledException("Stroke is stopping");if(!compiler.Start())throw new InvalidOperationException("脚本编译器未启动");activeCompiler=compiler;}var stdout=compiler.StandardOutput.ReadToEndAsync();var stderr=compiler.StandardError.ReadToEndAsync();if(!compiler.WaitForExit(30000)){compiler.Kill();compiler.WaitForExit();throw new InvalidOperationException("脚本编译超时");}string diagnostic=stdout.GetAwaiter().GetResult()+stderr.GetAwaiter().GetResult();if(compiler.ExitCode!=0||!File.Exists(output))throw new InvalidDataException(diagnostic.Length==0?"脚本编译失败":diagnostic);}finally{lock(compileGate)activeCompiler=null;File.Delete(sourcePath);}}
            return new ScriptPlan{assembly=output,configuration=config,functions=functions};
        }
    }
}
