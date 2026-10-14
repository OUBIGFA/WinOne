using System.Runtime.InteropServices;
using System.Windows.Automation;

// Opt-in integration helper. Every operation is scoped to the HWND and PID
// returned for this test's own result; it never searches unrelated windows.
internal static class ResultWindowAutomationProbe
{
    [StructLayout(LayoutKind.Sequential)] private struct Rect {public int Left,Top,Right,Bottom;}
    [StructLayout(LayoutKind.Sequential)] private struct Point {public int X,Y;}
    [DllImport("user32.dll")] private static extern bool ScreenToClient(IntPtr window,ref Point point);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);
    [DllImport("user32.dll")] private static extern bool GetWindowRect(IntPtr window,out Rect value);
    [DllImport("user32.dll")] private static extern uint GetDpiForWindow(IntPtr window);
    [DllImport("user32.dll")] private static extern bool SetWindowPos(IntPtr window,IntPtr after,int x,int y,int width,int height,uint flags);
    [DllImport("user32.dll")] private static extern bool IsWindow(IntPtr window);
    [DllImport("user32.dll")] private static extern bool PrintWindow(IntPtr window,IntPtr dc,uint flags);
    [DllImport("user32.dll")] private static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] private static extern bool SetForegroundWindow(IntPtr window);
    [DllImport("dwmapi.dll")] private static extern int DwmGetWindowAttribute(IntPtr window,int attribute,out uint value,int size);
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window,uint message,IntPtr w,IntPtr l);
    [DllImport("user32.dll",CharSet=CharSet.Unicode)] private static extern IntPtr FindWindowExW(IntPtr parent,IntPtr after,string className,string? title);

    internal static void Run(IntPtr window,int pid,string output,bool translate)
    {
        Check(window,pid);
        if(DwmGetWindowAttribute(window,33,out var corners,4)<0||corners!=2)throw new Exception("Result does not use native rounded corners");
        File.WriteAllText(Path.Combine(output,"frame-policy.json"),System.Text.Json.JsonSerializer.Serialize(new{nativeCorners=true,cornerPreference=corners}));
        Console.WriteLine($"Result focus at start: foreground={GetForegroundWindow()}, result={window}");
        if(GetForegroundWindow()!=window)throw new Exception("Capture did not transfer focus to the result window");
        var root=AutomationElement.FromHandle(window);
        Invoke(root,"固定结果窗");
        Wait(()=>Has(root,"取消固定"),"pin applied");
        Wait(()=>File.Exists(Path.Combine(output,"provider-waiting")),"provider request in flight");
        bool Working()=>Has(root,"正在处理…")||Has(root,"正在生成…");
        if(!Working()||!Has(root,"拖动结果窗"))throw new Exception("Busy result lost its drag handle");
        if(!GetWindowRect(window,out var before))throw new Exception("Result bounds unavailable");
        VerifyResizeEdges(window,before);
        if(!SetWindowPos(window,IntPtr.Zero,before.Left+35,before.Top+25,620,480,0x0004|0x0010))throw new Exception("Result move/resize failed");
        Wait(()=>GetWindowRect(window,out var after)&&after.Left==before.Left+35&&after.Top==before.Top+25&&after.Right-after.Left==620&&after.Bottom-after.Top==480,"independent result bounds");
        if(!Working())throw new Exception("Busy move/resize cancelled the request");
        File.WriteAllText(Path.Combine(output,"busy-resized.json"),System.Text.Json.JsonSerializer.Serialize(new{moved=true,resized=true,processing=true,resizeEdges=8}));
        var hidden=Environment.GetEnvironmentVariable("WINONE_TEXT_PIPELINE_HIDE_REASONING")=="1";
        Wait(()=>Has(root,translate?"你好":"He"),"first result text before completion");
        var initialModel=translate?"gemini-2.5-pro":"fixture-vision-model";
        CheckCaptureIdentityHidden(root,initialModel,translate?"你好":"He");
        var visibleAt=DateTime.UtcNow;
        using var timing=System.Text.Json.JsonDocument.Parse(File.ReadAllText(Path.Combine(output,"stream-sent.json")));
        var sentAt=DateTime.Parse(timing.RootElement.GetProperty("sentUtc").GetString()!,null,System.Globalization.DateTimeStyles.RoundtripKind).ToUniversalTime();
        var firstVisibleMs=(visibleAt-sentAt).TotalMilliseconds;
        if(!Working()||Has(root,"思考过程")||Has(root,"模型返回的测试思考"))throw new Exception("Initial result exposed reasoning or completed early");
        File.WriteAllText(Path.Combine(output,"stream-visible.json"),System.Text.Json.JsonSerializer.Serialize(new{beforeCompletion=true,initialReasoningHidden=true,firstVisibleMs}));
        Wait(()=>!Working(),"complete streamed result");
        CheckCaptureIdentityHidden(root,initialModel,translate?"你好，世界":"Hello world");
        File.WriteAllText(Path.Combine(output,"capture-identity-hidden.json"),System.Text.Json.JsonSerializer.Serialize(new{captureIdentityHidden=true,streamingAndCompleted=true}));
        ResizeComposer(root,window,output);
        Invoke(root,translate?"复制译文":"复制原文");
        AssertClipboard(translate ? "你好，世界" : "Hello world");
        Check(window,pid); // Copy must keep the result window alive.
        var copyBounds=Find(root,translate?"复制译文":"复制原文",true).Current.BoundingRectangle;
        if(Hit(window,(int)(copyBounds.Left+copyBounds.Width/2),(int)(copyBounds.Top+copyBounds.Height/2))!=1)throw new Exception("Copy action overlaps a resize or drag region");
        Send(root,"解释这句话");Wait(()=>Has(root,"第一"),"first conversation delta");
        if(hidden){if(Has(root,"思考过程"))throw new Exception("Disabled chat reasoning exposed a disclosure");}
        else{
            Wait(()=>Has(root,"思考过程"),"chat reasoning disclosure");
            if(Has(root,"模型返回的测试思考"))throw new Exception("Chat reasoning expanded by default");
            SetReasoningExpanded(root,true);Wait(()=>Has(root,"模型返回的测试思考"),"user-expanded chat reasoning");
            var collapse=Find(root,"收起思考过程",true).Current.BoundingRectangle;
            if(!GetWindowRect(window,out var bounds)||collapse.Top<bounds.Top||collapse.Bottom>bounds.Bottom)throw new Exception("Fixed reasoning collapse is not reachable");
            Invoke(root,"收起思考过程");Wait(()=>!Has(root,"模型返回的测试思考"),"collapse reasoning from composer");
        }
        File.WriteAllText(Path.Combine(output,"chat-stream-visible.json"),System.Text.Json.JsonSerializer.Serialize(new{beforeCompletion=true,reasoningCollapsed=true,expandedByUser=!hidden,collapsedFromComposer=!hidden,reasoningHidden=hidden}));
        Wait(()=>Has(root,"第一轮回答"),"first AI answer");
        Wait(()=>Has(root,initialModel)&&Has(root,"服务一"),"first reply uses the feature provider and model");
        Invoke(root,"复制回复");
        AssertClipboard("第一轮回答");
        Invoke(root,"切换供应商和模型");
        ChooseModel(root,"服务二","gpt-5");
        Send(root,"再简短一些");Wait(()=>Has(root,"第二轮回答"),"second AI answer");
        Wait(()=>Has(root,"Mermaid 图表"),"rendered Mermaid flowchart");
        Invoke(root,"显示或隐藏源码");
        Invoke(root,"复制图表源码");
        Invoke(root,"显示或隐藏源码");
        Wait(()=>Has(root,initialModel)&&Has(root,"gpt-5")&&Has(root,"服务一")&&Has(root,"服务二"),"historical reply provider and model labels");
        Wait(()=>!Working(),"completed second reply");
        var secondAnswer=File.ReadAllText(Path.Combine(output,"second-answer.txt"));
        foreach(var reply in new[]{(index:0,text:"第一轮回答"),(index:1,text:secondAnswer)}) {
            Wait(()=>{
                var buttons=root.FindAll(TreeScope.Descendants,new AndCondition(new PropertyCondition(AutomationElement.NameProperty,"复制回复"),new PropertyCondition(AutomationElement.ControlTypeProperty,ControlType.Button)));
                if(buttons.Count!=2)return false;
                ((InvokePattern)buttons[reply.index].GetCurrentPattern(InvokePattern.Pattern)).Invoke();return true;
            },"copy individual reply "+reply.index);
            AssertClipboard(reply.text);
        }
        File.WriteAllText(Path.Combine(output,"reply-copy.json"),System.Text.Json.JsonSerializer.Serialize(new{initialResultCopied=true,individualRepliesCopied=2,markdownPreserved=true}));
        // Exercise the rendered native window, not only the headless layout.
        // The reading column and its actions must remain centered after growing.
        var scale=GetDpiForWindow(window)/96.0;
        var wideWidth=(int)(1200*scale);var wideHeight=(int)(700*scale);
        if(!SetWindowPos(window,IntPtr.Zero,before.Left,before.Top,wideWidth,wideHeight,0x0004|0x0010))throw new Exception("Wide result resize failed");
        Wait(()=>{
            if(!GetWindowRect(window,out var bounds)||bounds.Right-bounds.Left!=wideWidth)return false;
            var answer=Find(root,"第二轮回答").Current.BoundingRectangle;
            return Math.Abs(answer.Left+answer.Width/2-(bounds.Left+bounds.Right)/2.0)<=2*scale&&answer.Width>=720*scale&&answer.Width<=920*scale;
        },"centered wide reading column");
        File.WriteAllText(Path.Combine(output,"wide-layout.json"),System.Text.Json.JsonSerializer.Serialize(new{centered=true,width=wideWidth,height=wideHeight,scale}));
        using(var bitmap=new Bitmap(wideWidth,wideHeight)){
            using(var graphics=Graphics.FromImage(bitmap)){var dc=graphics.GetHdc();try{if(!PrintWindow(window,dc,1))throw new Exception("Wide result screenshot failed");}finally{graphics.ReleaseHdc(dc);}}
            bitmap.Save(Path.Combine(output,"wide-result-window.png"));
        }
        if(!SetWindowPos(window,IntPtr.Zero,before.Left+35,before.Top+25,620,480,0x0004|0x0010))throw new Exception("Result restore size failed");
        if(Environment.GetEnvironmentVariable("WINONE_RESULT_MANUAL_CHECK")=="1"){
            File.WriteAllText(Path.Combine(output,"manual-ready.json"),System.Text.Json.JsonSerializer.Serialize(new{window=window.ToInt64(),pid}));
            var deadline=DateTime.UtcNow.AddMinutes(4);
            while(!File.Exists(Path.Combine(output,"manual-continue"))){if(DateTime.UtcNow>deadline)throw new Exception("Manual result check was not completed");Thread.Sleep(100);}
        }
        Invoke(root,"复制原文");
        Check(window,pid);
        using(var bitmap=new Bitmap(620,480)){
            using(var graphics=Graphics.FromImage(bitmap)){var dc=graphics.GetHdc();try{if(!PrintWindow(window,dc,1))throw new Exception("Native result screenshot failed");}finally{graphics.ReleaseHdc(dc);}}
            bitmap.Save(Path.Combine(output,"result-window.png"));
        }
        // UIA Invoke can operate on a background window. A user clicking the
        // pin control first activates the result; reproduce that focus boundary
        // before checking the subsequent outside-click dismissal.
        Find(root,"取消固定",true).SetFocus();Wait(()=>GetForegroundWindow()==window,"result focused before unpin");
        Invoke(root,"取消固定");
        Wait(()=>Has(root,"固定结果窗"),"unpin applied before leaving the window");
        Console.WriteLine($"Result focus before dismissal: foreground={GetForegroundWindow()}, result={window}");
    }
    internal static void Check(IntPtr window,int pid){GetWindowThreadProcessId(window,out var current);if(current!=(uint)pid||!IsWindow(window))throw new Exception("Owned result identity changed");}
    private static void CheckCaptureIdentityHidden(AutomationElement root,string model,string text){
        // Capture does not show identity in its result block. The model may
        // still appear in the composer, so check only above the result body.
        var body=Find(root,text).Current.BoundingRectangle;
        foreach(var name in new[]{"服务一",model}){
            foreach(AutomationElement identity in root.FindAll(TreeScope.Descendants,new PropertyCondition(AutomationElement.NameProperty,name))){
                var bounds=identity.Current.BoundingRectangle;
                if(!identity.Current.IsOffscreen&&!bounds.IsEmpty&&bounds.Bottom<=body.Top+1)throw new Exception("Capture result unexpectedly shows provider/model: "+name);
            }
        }
    }
    internal static void FocusOwned(IntPtr window){
        AutomationElement.FromHandle(window).SetFocus();
        // UIA keyboard focus alone does not guarantee top-level activation.
        // Verify this fixture's own window before sending canvas input.
        if(!SetForegroundWindow(window)&&GetForegroundWindow()!=window)throw new Exception("Owned capture fixture could not become foreground");
    }
    private static int Hit(IntPtr window,int x,int y)=>SendMessageW(window,0x0084,IntPtr.Zero,new IntPtr((y<<16)|(x&0xffff))).ToInt32();
    private static void VerifyResizeEdges(IntPtr window,Rect rect){
        var left=rect.Left+2;var right=rect.Right-3;var top=rect.Top+2;var bottom=rect.Bottom-3;var x=(left+right)/2;var y=(top+bottom)/2;
        foreach(var point in new[]{(left,y,10),(right,y,11),(x,top,12),(x,bottom,15),(left,top,13),(right,top,14),(left,bottom,16),(right,bottom,17)})
            {var hit=Hit(window,point.Item1,point.Item2);if(hit!=point.Item3)throw new Exception($"Native result resize edge missing: expected {point.Item3}, got {hit}, point={point.Item1},{point.Item2}, rect={rect.Left},{rect.Top},{rect.Right},{rect.Bottom}");}
    }
    internal static void FocusOutsideAndWaitClosed(IntPtr outside,IntPtr result){
        AutomationElement.FromHandle(outside).SetFocus();
        Wait(()=>GetForegroundWindow()==outside,"owned outside window focus");
        Wait(()=>!IsWindow(result),"outside-focus dismissal");
    }
    private static AutomationElement Find(AutomationElement root,string name,bool button=false)
    {
        AutomationElement? value=null;
        Wait(()=>{value=root.FindFirst(TreeScope.Descendants,button?new AndCondition(new PropertyCondition(AutomationElement.NameProperty,name),new PropertyCondition(AutomationElement.ControlTypeProperty,ControlType.Button)):new PropertyCondition(AutomationElement.NameProperty,name));return value is not null;},"control "+name);
        return value!;
    }
    private static void Invoke(AutomationElement root,string name){
        // A completed OCR request replaces the busy subtree. Re-query that
        // transient UIA element instead of retaining a provider from an old frame.
        Wait(()=>{try{var button=Find(root,name,true);if(!button.Current.IsEnabled)return false;((InvokePattern)button.GetCurrentPattern(InvokePattern.Pattern)).Invoke();return true;}catch(ElementNotAvailableException){return false;}},"invoke "+name);
    }
    private static bool Has(AutomationElement root,string text)=>root.FindFirst(TreeScope.Descendants,new PropertyCondition(AutomationElement.NameProperty,text)) is not null;
    private static void AssertClipboard(string expected){
        // UIA runs on an MTA worker; read OLE clipboard on its required STA.
        // The enclosing native probe restores the user's clipboard at exit.
        Wait(()=>{
            string? actual=null;Exception? failure=null;
            var thread=new Thread(()=>{try{actual=Clipboard.GetText();}catch(Exception error){failure=error;}});
            thread.SetApartmentState(ApartmentState.STA);thread.Start();
            if(!thread.Join(3000))throw new Exception("Clipboard read did not return");
            if(failure is not null)throw failure;
            return actual==expected;
        },"exact copied answer");
    }
    private static void ChooseModel(AutomationElement root,string provider,string model){
        // The shared picker searches one grouped list; selecting commits both
        // provider and model at once, with no intermediate route change.
        const string control="搜索供应商或模型";
        Wait(()=>{var current=Find(root,control);if(!current.TryGetCurrentPattern(ValuePattern.Pattern,out var pattern))return false;((ValuePattern)pattern).SetValue(provider+" "+model);return true;},"filter provider and model");
        Wait(()=>Has(root,model),"matching model");
        var previous=Find(root,"切换供应商和模型",true).Current.HelpText;
        SetPickerGroupExpanded(root,provider,false);
        Wait(()=>!Has(root,model),"provider models collapsed");
        if(Find(root,"切换供应商和模型",true).Current.HelpText!=previous)throw new Exception("Collapsing a provider changed the route");
        SetPickerGroupExpanded(root,provider,true);
        Wait(()=>Has(root,model),"provider models expanded");
        Console.WriteLine("Native provider disclosure collapsed and expanded without changing the route");
        Invoke(root,model);
        Wait(()=>!Has(root,control)&&Find(root,"切换供应商和模型",true).Current.HelpText==provider+" · "+model,"apply route and close picker");
    }
    private static void SetPickerGroupExpanded(AutomationElement root,string provider,bool expanded){
        // Streaming frames can retire a UIA provider between lookup and action.
        // Re-read the disclosure state; Expand/Collapse are idempotent, so an
        // uncertain action is verified rather than replayed as a toggle.
        Wait(()=>{
            var pattern=(ExpandCollapsePattern)Find(root,provider,true).GetCurrentPattern(ExpandCollapsePattern.Pattern);
            if(pattern.Current.ExpandCollapseState==(expanded?ExpandCollapseState.Expanded:ExpandCollapseState.Collapsed))return true;
            if(expanded)pattern.Expand();else pattern.Collapse();
            return false;
        },"provider disclosure "+provider);
    }
    private static void Send(AutomationElement root,string value){((ValuePattern)Find(root,"继续提问").GetCurrentPattern(ValuePattern.Pattern)).SetValue(value);Invoke(root,"发送");}
    private static void SetReasoningExpanded(AutomationElement root,bool expanded){
        // Disclosure controls expose ExpandCollapse, rather than Invoke.
        var pattern=(ExpandCollapsePattern)Find(root,"思考过程",true).GetCurrentPattern(ExpandCollapsePattern.Pattern);
        if(expanded)pattern.Expand();else pattern.Collapse();
        Wait(()=>((ExpandCollapsePattern)Find(root,"思考过程",true).GetCurrentPattern(ExpandCollapsePattern.Pattern)).Current.ExpandCollapseState==(expanded?ExpandCollapseState.Expanded:ExpandCollapseState.Collapsed),"reasoning disclosure state");
    }
    private static void Wait(Func<bool> ready,string label){
        var end=DateTime.UtcNow.AddSeconds(15);Exception? unavailable=null;
        while(true){
            // mygo replaces accessibility descendants on streamed frames. A
            // stale provider means retry the predicate, never treat it as ready.
            try{if(ready())return;}catch(ElementNotAvailableException error){unavailable=error;}
            if(DateTime.UtcNow>end)throw new Exception("Result integration timed out: "+label,unavailable);
            Thread.Sleep(30);
        }
    }
    private static void ResizeComposer(AutomationElement root,IntPtr window,string output){
        var input=Find(root,"继续提问");var height=input.Current.BoundingRectangle.Height;
        ((ValuePattern)input.GetCurrentPattern(ValuePattern.Pattern)).SetValue("保留\n草稿");
        var divider=Find(root,"Divider").Current.BoundingRectangle;
        var surface=FindWindowExW(window,IntPtr.Zero,"MyGoSurface",null);if(surface==IntPtr.Zero)throw new Exception("Owned input surface missing");
        var point=new Point{X=(int)(divider.Left+divider.Width/2),Y=(int)(divider.Top+divider.Height/2)};
        if(!ScreenToClient(surface,ref point))throw new Exception("Composer coordinates unavailable");
        IntPtr At(int x,int y)=>new((y<<16)|(x&0xffff));
        SendMessageW(surface,0x0200,IntPtr.Zero,At(point.X,point.Y));
        SendMessageW(surface,0x0201,new IntPtr(1),At(point.X,point.Y));
        SendMessageW(surface,0x0200,new IntPtr(1),At(point.X,point.Y-60));
        // A real drag spans UI frames. Keep capture held until the resize is
        // observable instead of sending down/move/up before any frame runs.
        try{Wait(()=>Find(root,"继续提问").Current.BoundingRectangle.Height>height+40,"dragged composer height");}
        finally{SendMessageW(surface,0x0202,IntPtr.Zero,At(point.X,point.Y-60));}
        if(((ValuePattern)Find(root,"继续提问").GetCurrentPattern(ValuePattern.Pattern)).Current.Value!="保留\n草稿")throw new Exception("Composer resize lost multiline draft");
        ((ValuePattern)Find(root,"继续提问").GetCurrentPattern(ValuePattern.Pattern)).SetValue("");
        File.WriteAllText(Path.Combine(output,"composer-resized.json"),System.Text.Json.JsonSerializer.Serialize(new{draggedUp=true,multilinePreserved=true}));
    }
}
