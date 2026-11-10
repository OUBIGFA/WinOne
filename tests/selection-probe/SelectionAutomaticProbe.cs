using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;
using System.Windows.Automation;

internal static partial class SelectionPipelineProbe
{
    [DllImport("user32.dll")] static extern void mouse_event(uint flags,uint x,uint y,uint data,UIntPtr extra);
    static async Task CheckAutomaticSelection(int hostPid,string pipe,Form controller,string output)
    {
        var fixture=Environment.GetEnvironmentVariable("WINONE_SELECTION_AUTOMATIC_FIXTURE")!;
        var settings=Environment.GetEnvironmentVariable("WINONE_SELECTION_AUTOMATIC_MODES")=="settings-double";
        if(settings){
            var configuration=await Call(pipe,"ai.configuration.get",new{});
            await Call(pipe,"ai.configuration.save",new{configuration=new{revision=configuration.GetProperty("revision").GetUInt64(),engine="google",ocrEngine="local",target="zh-CN",prompt="",showReasoning=true,providers=new[]{new{id="fixture",name="测试服务",authMode="api-key",protocol="chat",baseUrl="http://127.0.0.1:1/v1",models=new[]{"fixture"}}},tasks=new Dictionary<string,object>{{"default",new{provider="fixture",model="fixture"}}}},keys=new Dictionary<string,string>{{"fixture","fixture-key"}}});
        }
        var settingsExe=Path.Combine(Environment.GetEnvironmentVariable("WINONE_SELECTION_PIPELINE_BUNDLE")!,"WinOne.exe");
        var start=new ProcessStartInfo(settings?settingsExe:fixture){UseShellExecute=false,CreateNoWindow=true};
        if(settings)foreach(var argument in new[]{"--standard-session","--pipe",@"\\.\pipe\"+pipe,"--data-dir",Path.Combine(output,"data"),"--open-settings"})start.ArgumentList.Add(argument);
        using var app=Process.Start(start)??throw new Exception("MyGo source not started");
        try {
            IntPtr window=IntPtr.Zero;
            await Until(()=>{app.Refresh();window=app.MainWindowHandle;return window!=IntPtr.Zero;},"MyGo source");
            File.WriteAllText(Path.Combine(output,"source.json"),JsonSerializer.Serialize(new{pid=app.Id,window=window.ToInt64(),settings}));
            controller.Hide();SetForegroundWindow(window);
            try{await Until(()=>GetForegroundWindow()==window,"source foreground");}catch{Save(window,Path.Combine(output,"source-focus-failure.png"),true);throw;}
            var root=AutomationElement.FromHandle(window);
            if(settings) await Task.Run(()=>{
                WaitControl(root,"AI 服务");Click(root,"AI 服务");WaitControl(root,"自定义供应商");Click(root,"自定义供应商");
                try{WaitControl(root,"API 基础地址");}catch{Save(window,Path.Combine(output,"settings-page.png"),true);File.WriteAllText(Path.Combine(output,"settings-page.json"),JsonSerializer.Serialize(root.FindAll(TreeScope.Descendants,Condition.TrueCondition).Cast<AutomationElement>().Select(e=>e.Current.Name).ToArray()));throw;}
                ((ValuePattern)EditControl(root,"API 基础地址").GetCurrentPattern(ValuePattern.Pattern)).SetValue("WinOneSelection");
            });
            var samples=new List<object>();
            var modes=(Environment.GetEnvironmentVariable("WINONE_SELECTION_AUTOMATIC_MODES")??"input-double,paragraph-double,paragraph-triple,input-drag").Split(',');
            foreach(var mode in modes){
                SetForegroundWindow(window);
                var element=await Task.Run(()=>settings?EditControl(root,"API 基础地址"):root.FindFirst(TreeScope.Descendants,new PropertyCondition(AutomationElement.NameProperty,mode.StartsWith("input")?"Selection input":"WinOne paragraph selection")))??throw new Exception("Selection control missing");
                var rect=element.Current.BoundingRectangle;
                // Click different content first, breaking the previous click chain.
                SetCursorPos((int)rect.Right-10,(int)rect.Bottom+15);mouse_event(2,0,0,0,UIntPtr.Zero);mouse_event(4,0,0,0,UIntPtr.Zero);
                await Task.Delay(60);
                int x=(int)rect.Left+(mode.StartsWith("input")?70:16),y=(int)(rect.Top+rect.Height/2);
                var clock=Stopwatch.StartNew();
                if(mode.EndsWith("keyboard")){
                    await Task.Run(()=>element.SetFocus());
                    Keys(new[]{(0x11,false),(0x41,false),(0x11,true),(0x41,true)});
                }else if(mode.EndsWith("drag")){
                    x=(int)rect.Left+8;SetCursorPos(x,y);mouse_event(2,0,0,0,UIntPtr.Zero);
                    await Task.Delay(30);SetCursorPos((int)rect.Right-10,y);mouse_event(4,0,0,0,UIntPtr.Zero);
                }else{
                    DoubleClickAt(x,y);
                    if(mode.EndsWith("triple")){await Task.Delay(35);mouse_event(2,0,0,0,UIntPtr.Zero);mouse_event(4,0,0,0,UIntPtr.Zero);}
                }
                IntPtr toolbar=IntPtr.Zero;
                await Until(()=>{toolbar=FindWindow(Child(hostPid,"WinOne.Results.exe"));return toolbar!=IntPtr.Zero;},mode+" toolbar");
                var strip=AutomationElement.FromHandle(toolbar);await Task.Run(()=>WaitControl(strip,"复制"));
                await Until(()=>{var copy=Control(strip,"复制");return copy.Current.IsEnabled&&copy.TryGetClickablePoint(out _);},mode+" copy button ready");
                var elapsed=clock.Elapsed.TotalMilliseconds;
                if(GetForegroundWindow()!=window)throw new Exception("Toolbar stole source focus");
                Save(toolbar,Path.Combine(output,mode+".png"),includeFrame:true);
                await Task.Run(()=>Click(strip,"复制"));
                await Until(()=>!IsWindow(toolbar),"toolbar copied and closed");
                var expected=mode switch{"input-double" or "settings-double"=>"WinOneSelection","paragraph-double"=>"WinOne","paragraph-triple"=>"WinOne paragraph selection",_=>"Before WinOneSelection after"};
                var text="";
                await Until(()=>{text=Clipboard.GetText();return text==expected;},mode+" clipboard write");
                if(text!=expected)throw new Exception(mode+" copied wrong text: "+text);
                samples.Add(new{mode,milliseconds=elapsed,exactText=true,sourceKeptFocus=true});
                File.WriteAllText(Path.Combine(output,"samples.json"),JsonSerializer.Serialize(samples,new JsonSerializerOptions{WriteIndented=true}),new UTF8Encoding(false));
            }
            File.WriteAllText(Path.Combine(output,"automatic.json"),JsonSerializer.Serialize(new{passed=true,samples},new JsonSerializerOptions{WriteIndented=true}),new UTF8Encoding(false));
        }finally{if(!app.HasExited)app.Kill(true);}
    }
}
