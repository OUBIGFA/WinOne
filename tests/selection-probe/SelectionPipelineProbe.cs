using System.Diagnostics;
using System.IO.Pipes;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Windows.Automation;
using WinOne.Capture;

// Opt-in end-to-end test. Input targets only this fixture, and AI requests go
// only to the loopback server supplied by the Go integration test.
internal static partial class SelectionPipelineProbe
{
    [StructLayout(LayoutKind.Sequential)] struct Point { public int X,Y; }
    [StructLayout(LayoutKind.Sequential)] struct Rect { public int Left,Top,Right,Bottom; }
    [StructLayout(LayoutKind.Explicit,Size=32)] struct InputData {
        [FieldOffset(0)] public int X; [FieldOffset(4)] public int Y;
        [FieldOffset(8)] public uint MouseData; [FieldOffset(12)] public uint MouseFlags;
        [FieldOffset(0)] public ushort Key; [FieldOffset(4)] public uint KeyFlags;
        [FieldOffset(24)] public UIntPtr Extra;
    }
    [StructLayout(LayoutKind.Sequential)] struct Input {public uint Type;public InputData Data;}
    [StructLayout(LayoutKind.Sequential,CharSet=CharSet.Unicode)] struct ProcessEntry {public uint Size,Usage,Pid;public UIntPtr Heap;public uint Module,Threads,Parent;public int Priority;public uint Flags;[MarshalAs(UnmanagedType.ByValTStr,SizeConst=260)]public string Name;}
    delegate bool EnumProc(IntPtr hwnd,IntPtr parameter);
    [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc callback,IntPtr parameter);
    [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr window);
    [DllImport("user32.dll")] static extern bool IsWindow(IntPtr window);
    [DllImport("user32.dll",EntryPoint="GetWindowLongPtrW")] static extern IntPtr GetWindowLong(IntPtr window,int index);
    [DllImport("user32.dll")] static extern uint GetDoubleClickTime();
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);
    [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr window);
    [DllImport("user32.dll")] static extern bool SetCursorPos(int x,int y);
    [DllImport("user32.dll")] static extern bool GetCursorPos(out Point point);
    [DllImport("user32.dll",SetLastError=true)] static extern uint SendInput(uint count,Input[] inputs,int size);
    [DllImport("user32.dll")] static extern bool SetWindowPos(IntPtr window,IntPtr after,int x,int y,int width,int height,uint flags);
    [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr window,out Rect rectangle);
    [DllImport("user32.dll")] static extern bool PrintWindow(IntPtr window,IntPtr dc,uint flags);
    [DllImport("dwmapi.dll")] static extern int DwmGetWindowAttribute(IntPtr window,uint attribute,out uint value,int size);
    [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern int GetClassNameW(IntPtr window,StringBuilder name,int length);
    [DllImport("user32.dll")] static extern bool PostMessageW(IntPtr window,uint message,IntPtr w,IntPtr l);
    [DllImport("kernel32.dll")] static extern IntPtr CreateToolhelp32Snapshot(uint flags,uint pid);
    [DllImport("kernel32.dll",CharSet=CharSet.Unicode)] static extern bool Process32FirstW(IntPtr snapshot,ref ProcessEntry entry);
    [DllImport("kernel32.dll",CharSet=CharSet.Unicode)] static extern bool Process32NextW(IntPtr snapshot,ref ProcessEntry entry);
    [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr handle);

    internal static int Run(string output) {
        var quickOnly=Environment.GetEnvironmentVariable("WINONE_SELECTION_QUICK_HANDOFF")=="1";
        var bundle=Environment.GetEnvironmentVariable("WINONE_SELECTION_PIPELINE_BUNDLE")??throw new Exception("Set the explicit bundle path");
        var url=Environment.GetEnvironmentVariable("WINONE_SELECTION_PIPELINE_URL")??throw new Exception("Set a loopback fixture URL");
        if(!Uri.TryCreate(url,UriKind.Absolute,out var address)||!address.IsLoopback||address.Scheme!="http")throw new Exception("Only loopback fixture calls are allowed");
        if(Directory.Exists(output))throw new Exception("Choose a new output directory");
        var data=Path.Combine(output,"data");Directory.CreateDirectory(data);
        var theme=Environment.GetEnvironmentVariable("WINONE_SELECTION_PIPELINE_THEME")=="dark"?"dark":"light";
        File.WriteAllText(Path.Combine(data,"settings.json"),JsonSerializer.Serialize(new{schemaVersion=1,theme,language="zh-CN",captureShortcut="",pinShortcut="",ocrShortcut="",translateShortcut="",selectionShortcut=""}));
        if(quickOnly) File.WriteAllText(Path.Combine(data,"module-state.json"),JsonSerializer.Serialize(new{schemaVersion=3,revision=0,modules=new Dictionary<string,object>{{"capture",new{enabled=false,pendingStop=false}},{"always-on-top",new{enabled=false,pendingStop=false}},{"wintray",new{enabled=false,pendingStop=false}},{"selection",new{enabled=true,pendingStop=false}},{"quick-assistant",new{enabled=true,pendingStop=false}}}}));
        var pipe="WinOne-selection-test-"+Guid.NewGuid().ToString("N");
        var info=new ProcessStartInfo(Path.Combine(bundle,"WinOne.Host.exe")){UseShellExecute=false,CreateNoWindow=true,RedirectStandardError=true};
        if(Environment.GetEnvironmentVariable("WINONE_SELECTION_PIPELINE_DIAGNOSTICS")=="1")info.Environment["WINONE_SELECTION_DIAGNOSTICS"]="1";
        foreach(var value in new[]{"--standard-session","--pipe",@"\\.\pipe\"+pipe,"--data-dir",data})info.ArgumentList.Add(value);
        using var host=Process.Start(info)??throw new Exception("Host did not start");
        host.ErrorDataReceived+=(_,e)=>{if(e.Data is not null)File.AppendAllText(Path.Combine(output,"host.log"),e.Data+Environment.NewLine);};host.BeginErrorReadLine();
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);Application.EnableVisualStyles();
        var clipboard=quickOnly?null:SnapshotClipboard();
        using var form=new Form{Text="WinOne selection pipeline fixture",Width=700,Height=360,TopMost=true};
        using var source=new TextBox{Multiline=true,Dock=DockStyle.Fill,Text="Selected content",Font=new Font("Segoe UI",20)};form.Controls.Add(source);
        Exception? failure=null;int resultPid=0;Process? settingsWindow=null;
        form.Shown+=async(_,_)=>{
            try {
                _=await Call(pipe,"snapshot",new{});
                if(quickOnly){await CheckQuickHandoff(host.Id,pipe,source,form,output);return;}
                var configuration=await Call(pipe,"ai.configuration.get",new{});
                await Call(pipe,"ai.configuration.save",new{configuration=new{revision=configuration.GetProperty("revision").GetUInt64(),engine="google",ocrEngine="local",target="zh-CN",prompt="",showReasoning=true,providers=new[]{new{id="fixture",name="测试服务",authMode="api-key",protocol="chat",baseUrl=url,models=new[]{"fixture-translate","fixture-chat"}},new{id="custom-provider",name="自定义服务",authMode="api-key",protocol="chat",baseUrl=url+"/custom",models=new[]{"custom-model"}}},tasks=new Dictionary<string,object>{{"default",new{provider="fixture",model="fixture-translate"}},{"chat",new{provider="fixture",model="fixture-chat"}}}},keys=new Dictionary<string,string>{{"fixture","fixture-key"},{"custom-provider","custom-key"}}});
                if(Environment.GetEnvironmentVariable("WINONE_SELECTION_SETTINGS_DOUBLE_CLICK")=="1") {
                    var start=new ProcessStartInfo(Path.Combine(bundle,"WinOne.exe")){UseShellExecute=false};
                    foreach(var argument in new[]{"--standard-session","--pipe",@"\\.\pipe\"+pipe,"--data-dir",data,"--open-settings"})start.ArgumentList.Add(argument);
                    settingsWindow=Process.Start(start)??throw new Exception("Settings window did not start");
                    form.Hide();IntPtr settingsHandle=IntPtr.Zero;
                    await Until(()=>{settingsHandle=FindWindow(settingsWindow.Id);return settingsHandle!=IntPtr.Zero;},"owned settings window");
                    var settingsRoot=AutomationElement.FromHandle(settingsHandle);
                    if(!SetForegroundWindow(settingsHandle)&&GetForegroundWindow()!=settingsHandle)throw new Exception("Owned settings window could not become foreground");
                    await Until(()=>GetForegroundWindow()==settingsHandle,"owned settings foreground");
                    await Task.Run(()=>{WaitControl(settingsRoot,"AI服务");Click(settingsRoot,"AI服务");WaitControl(settingsRoot,"服务名称");});
                    var name=EditControl(settingsRoot,"服务名称");var value=(ValuePattern)name.GetCurrentPattern(ValuePattern.Pattern);var original=value.Current.Value;
                    await Task.Run(()=>value.SetValue("SelectionCheck"));
                    var bounds=name.Current.BoundingRectangle;Save(settingsHandle,Path.Combine(output,"settings-before-double-click.png"),true);DoubleClickAt((int)bounds.Left+24,(int)(bounds.Top+bounds.Height/2));
                    await Task.Delay(100);
                    var selectedText="";if(name.TryGetCurrentPattern(TextPattern.Pattern,out var textPattern))selectedText=string.Join("",((TextPattern)textPattern).GetSelection().Select(range=>range.GetText(-1)));
                    File.WriteAllText(Path.Combine(output,"settings-selection-diagnostic.json"),JsonSerializer.Serialize(new{window=settingsHandle.ToInt64(),foreground=GetForegroundWindow().ToInt64(),bounds=new{bounds.Left,bounds.Top,bounds.Width,bounds.Height},value=value.Current.Value,selectedText,patterns=name.GetSupportedPatterns().Select(p=>p.ProgrammaticName)}));
                    if(Environment.GetEnvironmentVariable("WINONE_SELECTION_DIAGNOSTIC_READ")=="1"){
                    var readerStart=new ProcessStartInfo(Path.Combine(bundle,"selection","WinOne.Selection.exe")){UseShellExecute=false,CreateNoWindow=true,RedirectStandardInput=true,RedirectStandardOutput=true,RedirectStandardError=true,StandardInputEncoding=new UTF8Encoding(false),StandardOutputEncoding=new UTF8Encoding(false)};
                    readerStart.Environment["WINONE_SELECTION_DIAGNOSTICS"]="1";
                    using(var reader=Process.Start(readerStart)??throw new Exception("Diagnostic reader did not start")){
                        await reader.StandardInput.WriteAsync(JsonSerializer.Serialize(new{window=(ulong)settingsHandle,pid=settingsWindow.Id}));reader.StandardInput.Close();
                        var readerOutput=reader.StandardOutput.ReadToEndAsync();var readerError=reader.StandardError.ReadToEndAsync();
                        try{await reader.WaitForExitAsync().WaitAsync(TimeSpan.FromSeconds(3));}catch{reader.Kill(true);throw;}
                        File.WriteAllText(Path.Combine(output,"settings-reader.json"),await readerOutput,new UTF8Encoding(false));File.WriteAllText(Path.Combine(output,"settings-reader.log"),await readerError,new UTF8Encoding(false));
                    }
                    }
                    IntPtr strip=IntPtr.Zero;try{await Until(()=>{strip=FindWindow(Child(host.Id,"WinOne.Results.exe"));return strip!=IntPtr.Zero;},"double-click toolbar inside WinOne settings");}catch{try{await Call(pipe,"selection.show",new{});}catch(Exception diagnostic){File.WriteAllText(Path.Combine(output,"manual-selection-diagnostic.txt"),diagnostic.ToString());}throw;}
                    var stripRoot=AutomationElement.FromHandle(strip);await Task.Run(()=>WaitControl(stripRoot,"复制"));
                    await Task.Delay((int)GetDoubleClickTime()+150);
                    if(!IsWindow(strip)||!IsWindowVisible(strip))throw new Exception("Double-click toolbar flashed and closed");
                    AssertToolWindow(strip);
                    await Task.Run(()=>Click(stripRoot,"复制"));await Until(()=>Clipboard.ContainsText()&&Clipboard.GetText()=="SelectionCheck","exact settings selection copied");
                    await Until(()=>!IsWindow(strip),"double-click toolbar copy closed");
                    if(!SetForegroundWindow(settingsHandle)&&GetForegroundWindow()!=settingsHandle)throw new Exception("Owned settings restore focus failed");
                    await Task.Run(()=>((ValuePattern)EditControl(settingsRoot,"服务名称").GetCurrentPattern(ValuePattern.Pattern)).SetValue(original));
                    PostMessageW(settingsHandle,0x10,IntPtr.Zero,IntPtr.Zero);
                    await settingsWindow.WaitForExitAsync().WaitAsync(TimeSpan.FromSeconds(15));
                    File.WriteAllText(Path.Combine(output,"settings-double-click.json"),JsonSerializer.Serialize(new{passed=true,exactSelection=true,stable=true,taskbarExcluded=true}));
                    form.Show();form.Activate();source.Focus();
                }
                if(Environment.GetEnvironmentVariable("WINONE_SELECTION_STARTUP_RUNS") is { } runsText && int.TryParse(runsText,out var runs) && runs>0){
                    if(runs>40)throw new Exception("Selection startup sample count is too large");
                    var samples=new List<double>();
                    for(var i=0;i<runs;i++){
                        var owned=form.Handle;await Task.Run(()=>AutomationElement.FromHandle(owned).SetFocus());source.Focus();source.Select(0,0);
                        await Until(()=>GetForegroundWindow()==form.Handle,"startup fixture foreground");
                        var clock=Stopwatch.StartNew();Keys(new[]{(0x11,false),(0x41,false),(0x41,true),(0x11,true)});
                        IntPtr toolbar=IntPtr.Zero;await Until(()=>{toolbar=FindWindow(Child(host.Id,"WinOne.Results.exe"));return toolbar!=IntPtr.Zero;},"startup toolbar window");
                        var toolbarRoot=AutomationElement.FromHandle(toolbar);await Task.Run(()=>WaitControl(toolbarRoot,"翻译"));await Until(()=>Control(toolbarRoot,"翻译").Current.IsEnabled,"startup toolbar ready");
                        samples.Add(clock.Elapsed.TotalMilliseconds);
                        var outside=source.PointToScreen(new System.Drawing.Point(source.Width-20,source.Height-20));ClickAt(outside.X,outside.Y);
                        await Until(()=>Child(host.Id,"WinOne.Results.exe")==0,"startup toolbar process released");
                    }
                    var warm=samples.Skip(1).Order().ToArray();var p95=warm.Length==0?0:warm[(int)Math.Ceiling(warm.Length*.95)-1];
                    File.WriteAllText(Path.Combine(output,"startup.json"),JsonSerializer.Serialize(new{firstMs=samples[0],warmP95Ms=p95,samples,scope="gesture to enabled UIA controls; file cache not cleared"}));
                    if(samples[0]>800 || warm.Length>0&&p95>300)throw new Exception($"Toolbar startup exceeded budget: first={samples[0]}, warm P95={p95}");
                }
                if(Environment.GetEnvironmentVariable("WINONE_SELECTION_IDLE_CHECK")=="1"){
                    File.WriteAllText(Path.Combine(output,"idle-ready.json"),JsonSerializer.Serialize(new{hostPid=host.Id}));
                    var limit=DateTime.UtcNow.AddMinutes(2);while(!File.Exists(Path.Combine(output,"idle-continue"))){if(DateTime.UtcNow>limit)throw new Exception("Idle resource check expired");await Task.Delay(100);}
                }
                for(var cycle=0;cycle<2;cycle++) {
                    if(cycle==1){var settings=JsonNode.Parse((await Call(pipe,"selection.settings.get",new{})).GetRawText())!;var actions=settings["actions"]!.AsArray();actions.Insert(0,JsonSerializer.SerializeToNode(new{id="custom-test",name="自定义功能",icon="code",kind="ai",prompt="Provide the custom fixture answer.",selection=new{provider="custom-provider",model="custom-model"},enabled=true}));await Call(pipe,"selection.settings.save",settings);}
                    var owned=form.Handle;
                    await Task.Run(()=>AutomationElement.FromHandle(owned).SetFocus());source.Focus();source.Select(0,0);
                    await Until(()=>GetForegroundWindow()==form.Handle,"fixture foreground");
                    // Cover pointer drag and keyboard selection through the real
                    // OS hooks; only this generated fixture receives the input.
                    if(cycle==0)await DragSelect(source);else Keys(new[]{(0x11,false),(0x41,false),(0x41,true),(0x11,true)});
                    IntPtr window=IntPtr.Zero;
                    await Until(()=>{resultPid=Child(host.Id,"WinOne.Results.exe");window=FindWindow(resultPid);return window!=IntPtr.Zero;},"selection toolbar");
                    var root=AutomationElement.FromHandle(window);
                    await Task.Run(()=>WaitControl(root,cycle==0?"翻译":"自定义功能"));
                    if(GetForegroundWindow()!=form.Handle)throw new Exception("Toolbar stole selection focus");
                    AssertToolWindow(window);
                    if(DwmGetWindowAttribute(window,33,out var cornerPreference,4)<0||cornerPreference!=2)throw new Exception("Toolbar did not use the shared native frame policy");
                    File.WriteAllText(Path.Combine(output,$"toolbar-frame-{cycle}.json"),JsonSerializer.Serialize(new{cornerPreference,nativeFrame=true}));
                    Save(window,Path.Combine(output,$"toolbar-{cycle}.png"),includeFrame:true);
                    if(cycle==1&&Control(root,"自定义功能").Current.BoundingRectangle.Left>=Control(root,"翻译").Current.BoundingRectangle.Left)throw new Exception("Custom action order was ignored");
                    if(Control(root,"复制").Current.BoundingRectangle.Left<=Control(root,"代码").Current.BoundingRectangle.Left)throw new Exception("Copy action was not placed last");
                    var hover=Control(root,cycle==0?"翻译":"自定义功能").Current.BoundingRectangle;
                    SetCursorPos((int)(hover.Left+hover.Width/2),(int)(hover.Top+hover.Height/2));
                    IntPtr tip=IntPtr.Zero;await Until(()=>{tip=FindWindow(resultPid,"tooltips_class32");return tip!=IntPtr.Zero&&IsWindowVisible(tip);},"native toolbar tooltip");
                    GetWindowRect(tip,out var tipBounds);GetWindowRect(window,out var stripBounds);
                    if(tipBounds.Bottom<=stripBounds.Bottom&&tipBounds.Top>=stripBounds.Top)throw new Exception("Tooltip remained clipped inside the toolbar");
                    if(GetForegroundWindow()!=form.Handle)throw new Exception("Tooltip stole source focus");
                    Save(tip,Path.Combine(output,$"tooltip-{cycle}.png"));
                    await Task.Run(()=>ClickAt((int)(hover.Left+hover.Width/2),(int)(hover.Top+hover.Height/2)));
                    var answer=cycle==0?"这是划词翻译结果":"自定义功能结果";
                    await Task.Run(()=>WaitControl(root,answer));
                    var expectedProvider=cycle==0?"测试服务":"自定义服务";var expectedModel=cycle==0?"fixture-translate":"custom-model";
                    var resultBounds=Control(root,answer).Current.BoundingRectangle;
                    foreach(var label in new[]{expectedProvider,expectedModel}){
                        var identity=Control(root,label);var identityBounds=identity.Current.BoundingRectangle;
                        if(identity.Current.IsOffscreen||identityBounds.IsEmpty||identityBounds.Bottom>resultBounds.Top+1)throw new Exception("Selection result identity missing above its body: "+label);
                    }
                    File.WriteAllText(Path.Combine(output,$"result-identity-{cycle}.json"),JsonSerializer.Serialize(new{provider=expectedProvider,model=expectedModel,aboveResult=true}));
                    AssertToolWindow(window);
                    foreach(var label in new[]{"原文","译文","Selected content"})if(Has(root,label))throw new Exception("Selection result repeated source UI: "+label);
                    if(!Has(root,"复制结果")||!Has(root,"继续提问"))throw new Exception("Shared result actions missing");
                    if(cycle==0) {
                        // Select inside the real result before pinning it. Its
                        // own toolbar must copy precisely and keep the source open.
                        foreach(var keyboard in new[]{false,true}) {
                        await Task.Run(()=>Click(root,answer));
                        if(keyboard) Keys(new[]{(0x11,false),(0x41,false),(0x41,true),(0x11,true)});
                        else {
                            var textBounds=Control(root,answer).Current.BoundingRectangle;
                            SetCursorPos((int)textBounds.Left+1,(int)(textBounds.Top+textBounds.Height/2));
                            if(SendInput(1,new[]{new Input{Type=0,Data=new InputData{MouseFlags=2}}},Marshal.SizeOf<Input>())!=1)throw new Exception("Result drag start failed");
                            SetCursorPos((int)textBounds.Right-1,(int)(textBounds.Top+textBounds.Height/2));
                            if(SendInput(1,new[]{new Input{Type=0,Data=new InputData{MouseFlags=4}}},Marshal.SizeOf<Input>())!=1)throw new Exception("Result drag end failed");
                        }
                        IntPtr nested=IntPtr.Zero;
                        await Until(()=>{nested=FindWindow(resultPid,null,window);return nested!=IntPtr.Zero;},"selection toolbar inside result");
                        var nestedRoot=AutomationElement.FromHandle(nested);
                        await Task.Run(()=>WaitControl(nestedRoot,"复制"));
                        Save(nested,Path.Combine(output,keyboard?"result-keyboard-selection-toolbar.png":"result-pointer-selection-toolbar.png"),includeFrame:true);
                        await Task.Run(()=>Click(nestedRoot,"复制"));
                        await Until(()=>!IsWindow(nested),"nested toolbar closed after copy");
                        if(!IsWindow(window)||Clipboard.GetText()!=answer)throw new Exception("Result selection lost source or copied wrong text");
                        }
                        File.WriteAllText(Path.Combine(output,"result-selection.json"),JsonSerializer.Serialize(new{passed=true,exactText=true,sourceKept=true}));
                    }
                    await Task.Run(()=>Click(root,"固定结果窗"));
                    GetWindowRect(window,out var before);if(!SetWindowPos(window,IntPtr.Zero,before.Left,before.Top,700,650,0x14))throw new Exception("Result resize failed");
                    await Task.Run(()=>{((ValuePattern)Control(root,"继续提问").GetCurrentPattern(ValuePattern.Pattern)).SetValue("继续解释");Click(root,"发送");WaitControl(root,"后续对话结果");});
                    if(!Has(root,"复制回复"))throw new Exception("AI reply lost its copy action");
                    if(cycle==1&&!Has(root,"复制代码"))throw new Exception("Code block lost its copy action");
                    await Task.Run(()=>Click(root,"复制结果"));
                    await Until(()=>Clipboard.ContainsText()&&Clipboard.GetText().StartsWith(answer,StringComparison.Ordinal),"copy complete result");
                    Save(window,Path.Combine(output,$"result-{cycle}.png"));
                    if(cycle==0){await Task.Run(()=>Click(root,"关闭"));await Until(()=>!IsWindow(window),"first result closed");}
                    else {
                        if(Environment.GetEnvironmentVariable("WINONE_SELECTION_MANUAL_CHECK")=="1") {
                            File.WriteAllText(Path.Combine(output,"manual-ready.json"),JsonSerializer.Serialize(new{hostPid=host.Id,resultPid,window=window.ToInt64()}));
                            var limit=DateTime.UtcNow.AddMinutes(3);while(!File.Exists(Path.Combine(output,"manual-continue"))){if(DateTime.UtcNow>limit)throw new Exception("Manual check expired");await Task.Delay(100);}
                        }
                        var modules=await Call(pipe,"modules.snapshot",new{});
                        await Call(pipe,"modules.setEnabled",new{id="selection",enabled=false,revision=modules.GetProperty("revision").GetUInt64()});
                        await Until(()=>!IsWindow(window),"module disable closed its result");
                    }
                    await Until(()=>Child(host.Id,"WinOne.Results.exe")==0,"result process released");
                }
                if(!RequestExit(host.Id))throw new Exception("Own Host tray exit unavailable");
                await host.WaitForExitAsync().WaitAsync(TimeSpan.FromSeconds(10));
                if(host.ExitCode!=0)throw new Exception("Host exit failed");
                File.WriteAllText(Path.Combine(output,"result.json"),JsonSerializer.Serialize(new{passed=true,pointerSelection=true,keyboardSelection=true,toolbarInactive=true,customAction=true,customOrder=true,selectionNotRepeated=true,conversationTurns=2,resultCopied=true,codeCopy=true,moduleDisabled=true,resultExited=true,hostExited=true}));
            }catch(Exception error){
                failure=error;
                // Capture only our synthetic fixture before teardown so input,
                // window-lifetime and request failures can be distinguished.
                var ownedResult=FindWindow(resultPid);GetCursorPos(out var cursor);
                var names=Array.Empty<string>();
                if(ownedResult!=IntPtr.Zero){Save(ownedResult,Path.Combine(output,"failure-window.png"),true);names=AutomationElement.FromHandle(ownedResult).FindAll(TreeScope.Descendants,Condition.TrueCondition).Cast<AutomationElement>().Select(element=>element.Current.Name).Where(name=>name.Length>0).ToArray();}
                File.WriteAllText(Path.Combine(output,"failure-window.json"),JsonSerializer.Serialize(new{resultPid,window=ownedResult.ToInt64(),foreground=GetForegroundWindow().ToInt64(),cursor=new{cursor.X,cursor.Y},names}));
            }finally{form.Close();}
        };
        try{Application.Run(form);}
        finally{try{if(!quickOnly){if(clipboard==null)Clipboard.Clear();else Clipboard.SetDataObject(clipboard,true,30,50);}}finally{if(settingsWindow is not null&&!settingsWindow.HasExited)settingsWindow.Kill(true);settingsWindow?.Dispose();if(!host.HasExited){RequestExit(host.Id);if(!host.WaitForExit(5000))host.Kill(true);}}}
        if(failure!=null){File.WriteAllText(Path.Combine(output,"failure.txt"),failure.ToString());Console.Error.WriteLine(failure);return 1;}
        Console.WriteLine(quickOnly ? "PASS: selection handoff, draft preservation, shared window, unchanged clipboard and module shutdown" : "PASS: global selection, toolbar, translation/custom action, shared conversation, copy, disable and exit");return 0;
    }
    static IDataObject? SnapshotClipboard(){
        var original=Clipboard.GetDataObject();if(original==null)return null;
        // Materialize formats before the test changes clipboard ownership.
        // A live IDataObject proxy may no longer render its original data later.
        var snapshot=new DataObject();
        foreach(var format in original.GetFormats(false)){
            var data=original.GetData(format,false);if(data==null)throw new Exception("Cannot preserve clipboard format: "+format);
            if(data is Image image)data=image.Clone();else if(data is MemoryStream stream)data=new MemoryStream(stream.ToArray());else if(data is byte[] bytes)data=bytes.ToArray();
            snapshot.SetData(format,false,data);
        }
        return snapshot;
    }
    static async Task<JsonElement> Call(string name,string method,object parameters){
        using var timeout=new CancellationTokenSource(TimeSpan.FromSeconds(20));
        using var pipe=new NamedPipeClientStream(".",name,PipeDirection.InOut,PipeOptions.Asynchronous);await pipe.ConnectAsync(timeout.Token);
        await PipeProtocol.WriteAsync(pipe,JsonSerializer.SerializeToUtf8Bytes(new{version=1,id=Guid.NewGuid().ToString("N"),method,@params=parameters}),timeout.Token);
        using var response=JsonDocument.Parse(await PipeProtocol.ReadAsync(pipe,timeout.Token));
        if(response.RootElement.TryGetProperty("error",out var error))throw new Exception(error.ToString());
        var result=response.RootElement.GetProperty("result").Clone();
        if(result.ValueKind==JsonValueKind.Object&&result.TryGetProperty("configurationJob",out var queued)){
            var deadline=DateTime.UtcNow.AddSeconds(20);
            while(true){var job=await Call(name,"configuration.jobs.get",new{id=queued.GetString()});if(job.GetProperty("state").GetString()!="running"){
                var completed=job.GetProperty("response");if(completed.TryGetProperty("error",out var failed))throw new Exception(failed.ToString());return completed.GetProperty("result").Clone();
            }if(DateTime.UtcNow>deadline)throw new Exception("Configuration job timed out");await Task.Delay(25);}
        }
        return result;
    }
    static async Task Until(Func<bool> ready,string label){var end=DateTime.UtcNow.AddSeconds(15);while(!ready()){if(DateTime.UtcNow>end)throw new Exception("Timed out: "+label);await Task.Delay(25);}}
    static AutomationElement Control(AutomationElement root,string name)=>root.FindFirst(TreeScope.Descendants,new PropertyCondition(AutomationElement.NameProperty,name))??throw new Exception("Control missing: "+name);
    static AutomationElement EditControl(AutomationElement root,string name)=>root.FindFirst(TreeScope.Descendants,new AndCondition(new PropertyCondition(AutomationElement.NameProperty,name),new PropertyCondition(AutomationElement.ControlTypeProperty,ControlType.Edit)))??throw new Exception("Text input missing: "+name);
    static bool Has(AutomationElement root,string name)=>root.FindFirst(TreeScope.Descendants,new PropertyCondition(AutomationElement.NameProperty,name))!=null;
    static void WaitControl(AutomationElement root,string name){var end=DateTime.UtcNow.AddSeconds(20);while(!Has(root,name)){if(DateTime.UtcNow>end)throw new Exception("Control did not appear: "+name);Thread.Sleep(25);}}
    static void Keys((int Key,bool Up)[] keys){var inputs=keys.Select(key=>new Input{Type=1,Data=new InputData{Key=(ushort)key.Key,KeyFlags=key.Up?2u:0u}}).ToArray();if(SendInput((uint)inputs.Length,inputs,Marshal.SizeOf<Input>())!=inputs.Length)throw new Exception("Owned key injection failed");}
    static async Task DragSelect(TextBox source){
        var first=source.PointToScreen(source.GetPositionFromCharIndex(0));var last=source.PointToScreen(source.GetPositionFromCharIndex(source.TextLength-1));
        SetCursorPos(first.X+1,first.Y+source.Font.Height/2);
        var down=new[]{new Input{Type=0,Data=new InputData{MouseFlags=2}}};if(SendInput(1,down,Marshal.SizeOf<Input>())!=1)throw new Exception("Owned drag start failed");
        try{SetCursorPos(last.X+source.Font.Height*2,last.Y+source.Font.Height/2);await Until(()=>source.SelectionLength==source.TextLength,"whole fixture selected by dragging");}
        finally{var up=new[]{new Input{Type=0,Data=new InputData{MouseFlags=4}}};SendInput(1,up,Marshal.SizeOf<Input>());}
    }
    static void Click(AutomationElement root,string name){var item=Control(root,name);var bounds=item.Current.BoundingRectangle;if(bounds.IsEmpty||!item.Current.IsEnabled)throw new Exception("Control not clickable: "+name);ClickAt((int)(bounds.Left+bounds.Width/2),(int)(bounds.Top+bounds.Height/2));}
    static void ClickAt(int x,int y){SetCursorPos(x,y);var inputs=new[]{new Input{Type=0,Data=new InputData{MouseFlags=2}},new Input{Type=0,Data=new InputData{MouseFlags=4}}};if(SendInput(2,inputs,Marshal.SizeOf<Input>())!=2)throw new Exception("Owned click failed");}
    static void DoubleClickAt(int x,int y){SetCursorPos(x,y);var inputs=new[]{new Input{Type=0,Data=new InputData{MouseFlags=2}},new Input{Type=0,Data=new InputData{MouseFlags=4}},new Input{Type=0,Data=new InputData{MouseFlags=2}},new Input{Type=0,Data=new InputData{MouseFlags=4}}};if(SendInput(4,inputs,Marshal.SizeOf<Input>())!=4)throw new Exception("Owned double-click failed");}
    static void AssertToolWindow(IntPtr window){var style=GetWindowLong(window,-20).ToInt64();if((style&0x80)==0||(style&0x40000)!=0)throw new Exception($"Selection window is taskbar-eligible: {style:x}");}
    static int Child(int parent,string name){var snapshot=CreateToolhelp32Snapshot(2,0);if(snapshot==new IntPtr(-1))throw new Exception("Process inventory failed");try{var entry=new ProcessEntry{Size=(uint)Marshal.SizeOf<ProcessEntry>(),Name=""};if(!Process32FirstW(snapshot,ref entry))return 0;do{if(entry.Parent==parent&&entry.Name==name)return (int)entry.Pid;}while(Process32NextW(snapshot,ref entry));return 0;}finally{CloseHandle(snapshot);}}
    static IntPtr FindWindow(int pid,string? className=null,IntPtr exclude=default){IntPtr found=IntPtr.Zero;EnumWindows((window,_)=>{GetWindowThreadProcessId(window,out var owner);if(owner!=pid||window==exclude)return true;if(className!=null){var name=new StringBuilder(256);GetClassNameW(window,name,256);if(name.ToString()!=className)return true;}else if(!IsWindowVisible(window))return true;found=window;return false;},IntPtr.Zero);return found;}
    static bool RequestExit(int pid){var tray=FindWindow(pid,"SystrayClass");return tray!=IntPtr.Zero&&PostMessageW(tray,0x111,new IntPtr(3),IntPtr.Zero);}
    static void Save(IntPtr window,string path,bool includeFrame=false){GetWindowRect(window,out var rect);using var bitmap=new Bitmap(rect.Right-rect.Left,rect.Bottom-rect.Top);using(var graphics=Graphics.FromImage(bitmap)){var dc=graphics.GetHdc();try{if(!PrintWindow(window,dc,includeFrame?0u:1u))throw new Exception("Window capture failed");}finally{graphics.ReleaseHdc(dc);}}bitmap.Save(path);}
}
