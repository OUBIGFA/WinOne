using System.Runtime.InteropServices;
using System.Text.Json;
using System.Windows.Automation;

internal static partial class SelectionPipelineProbe
{
    [DllImport("user32.dll")] static extern uint GetClipboardSequenceNumber();
    // Opt-in exercise of the real selection hook, reader, native toolbar and
    // chat host. The fixture never writes the clipboard or sends an AI request.
    static async Task CheckQuickHandoff(int hostPid,string pipe,TextBox source,Form form,string output)
    {
        var clipboardSequence=GetClipboardSequenceNumber();
        var quick=IntPtr.Zero;var resultPid=0;
        var selections=new[]{"Selected content","Second selection","Reopened selection"};
        for(var cycle=0;cycle<selections.Length;cycle++) {
            var previousQuick=quick;
            source.Text=selections[cycle];
            var owned=form.Handle;
            await Task.Run(()=>AutomationElement.FromHandle(owned).SetFocus());
            source.Focus();source.Select(0,0);
            await Until(()=>GetForegroundWindow()==owned,"owned source focus");
            Keys(new[]{(0x11,false),(0x41,false),(0x41,true),(0x11,true)});
            var toolbar=IntPtr.Zero;
            // A class-name match can include a window still being created.
            // Observe the visible toolbar before reading its accessibility tree.
            await Until(()=>{resultPid=Child(hostPid,"WinOne.Results.exe");toolbar=FindWindow(resultPid,"MyGoWindow",quick);return toolbar!=IntPtr.Zero&&IsWindowVisible(toolbar);},"selection toolbar for quick assistant");
            var strip=AutomationElement.FromHandle(toolbar);
            try { await Task.Run(()=>WaitControl(strip,"在快捷助手中打开")); }
            catch {
                Save(toolbar,Path.Combine(output,"quick-entry-missing.png"),includeFrame:true);
                var controls=strip.FindAll(TreeScope.Descendants,Condition.TrueCondition);
                File.WriteAllLines(Path.Combine(output,"quick-entry-controls.txt"),controls.Cast<AutomationElement>().Select(element=>$"{element.Current.ControlType.ProgrammaticName}: {element.Current.Name}"));
                throw;
            }
            await Until(()=>Control(strip,"在快捷助手中打开").Current.IsEnabled,"quick handoff enabled");
            Save(toolbar,Path.Combine(output,$"quick-toolbar-{cycle}.png"),includeFrame:true);
            await Task.Run(()=>Click(strip,"在快捷助手中打开"));
            // The pinned second handoff must preserve its draft and HWND. After
            // an explicit close, reacquire the new window instead of stale UIA.
            AutomationElement? chat=null;
            await Until(()=>{quick=FindWindow(resultPid,"MyGoWindow",toolbar);if(quick==IntPtr.Zero||!IsWindowVisible(quick))return false;try{chat=AutomationElement.FromHandle(quick);return true;}catch{return false;}},"quick chat window");
            if(chat is null)throw new Exception("Quick assistant window did not expose automation");
            await Task.Run(()=>WaitControl(chat,"输入消息"));
            // A visible window can still sit behind the source application.
            // Check the real foreground HWND before interacting with its input.
            await Until(()=>GetForegroundWindow()==quick,"quick assistant foreground activation");
            var expected=cycle==1?"Selected content\n\nSecond selection":selections[cycle];
            await Until(()=>((ValuePattern)EditControl(chat,"输入消息").GetCurrentPattern(ValuePattern.Pattern)).Current.Value==expected,"selected text in chat input");
            await Until(()=>!IsWindow(toolbar),"selection toolbar closed after handoff");
            if(Child(hostPid,"WinOne.Results.exe")!=resultPid)throw new Exception("Handoff replaced the shared result process");
            if(cycle==1&&quick!=previousQuick)throw new Exception("Pinned handoff replaced the quick assistant window");
            Save(quick,Path.Combine(output,$"quick-input-{cycle}.png"),includeFrame:true);
            if(cycle==0) {
                if((GetWindowLong(quick,-20).ToInt64()&0x8)!=0)throw new Exception("Quick assistant is topmost by default");
                await Task.Run(()=>Click(chat,"固定结果窗"));
                try { await Task.Run(()=>WaitControl(chat,"取消固定")); }
                catch {
                    Save(quick,Path.Combine(output,"quick-pin-failure.png"),includeFrame:true);
                    var controls=chat.FindAll(TreeScope.Descendants,Condition.TrueCondition);
                    File.WriteAllLines(Path.Combine(output,"quick-pin-controls.txt"),controls.Cast<AutomationElement>().Select(element=>$"{element.Current.ControlType.ProgrammaticName}: {element.Current.Name} {element.Current.BoundingRectangle}"));
                    File.WriteAllText(Path.Combine(output,"quick-pin-window.json"),JsonSerializer.Serialize(new{foreground=GetForegroundWindow().ToInt64(),quick=quick.ToInt64(),style=GetWindowLong(quick,-20).ToInt64()}));
                    throw;
                }
            } else if(cycle==1) {
                await Task.Run(()=>Click(chat,"关闭"));
                await Until(()=>!IsWindow(quick),"quick assistant explicitly closed");
                quick=IntPtr.Zero;
            }
        }
        if(GetClipboardSequenceNumber()!=clipboardSequence)throw new Exception("Selection handoff changed the system clipboard");
        var snapshot=await Call(pipe,"modules.snapshot",new{});
        await Call(pipe,"modules.setEnabled",new{id="quick-assistant",enabled=false,revision=snapshot.GetProperty("revision").GetUInt64()});
        await Until(()=>!IsWindow(quick)&&Child(hostPid,"WinOne.Results.exe")==0,"quick window and process closed");
        File.WriteAllText(Path.Combine(output,"quick-handoff.json"),JsonSerializer.Serialize(new{firstInput=selections[0],secondInput="Selected content\n\nSecond selection",reopenedInput=selections[2],foregroundActivated=true,defaultNotTopmost=true,existingDraftPreserved=true,reopenedDraftCleared=true,clipboardUnchanged=true,sharedProcessReused=true,disabledWindowClosed=true}));
    }
}
