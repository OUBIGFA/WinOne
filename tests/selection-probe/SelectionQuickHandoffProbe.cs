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
        for(var cycle=0;cycle<2;cycle++) {
            source.Text=cycle==0?"Selected content":"Second selection";
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
            // A quick assistant without pin closes when the next selection takes
            // focus, so every cycle must reacquire its current window handle.
            AutomationElement? chat=null;
            await Until(()=>{quick=FindWindow(resultPid,"MyGoWindow",toolbar);if(quick==IntPtr.Zero||!IsWindowVisible(quick))return false;try{chat=AutomationElement.FromHandle(quick);return true;}catch{return false;}},"quick chat window");
            if(chat is null)throw new Exception("Quick assistant window did not expose automation");
            await Task.Run(()=>WaitControl(chat,"输入消息"));
            var expected=cycle==0?"Selected content":"Selected content\n\nSecond selection";
            await Until(()=>((ValuePattern)EditControl(chat,"输入消息").GetCurrentPattern(ValuePattern.Pattern)).Current.Value==expected,"selected text in chat input");
            await Until(()=>!IsWindow(toolbar),"selection toolbar closed after handoff");
            if(Child(hostPid,"WinOne.Results.exe")!=resultPid)throw new Exception("Handoff replaced the shared result process");
            Save(quick,Path.Combine(output,$"quick-input-{cycle}.png"),includeFrame:true);
        }
        if(GetClipboardSequenceNumber()!=clipboardSequence)throw new Exception("Selection handoff changed the system clipboard");
        var snapshot=await Call(pipe,"modules.snapshot",new{});
        await Call(pipe,"modules.setEnabled",new{id="quick-assistant",enabled=false,revision=snapshot.GetProperty("revision").GetUInt64()});
        await Until(()=>!IsWindow(quick)&&Child(hostPid,"WinOne.Results.exe")==0,"quick window and process closed");
        File.WriteAllText(Path.Combine(output,"quick-handoff.json"),JsonSerializer.Serialize(new{firstInput="Selected content",secondInput="Selected content\n\nSecond selection",existingDraftPreserved=true,clipboardUnchanged=true,sharedProcessReused=true,disabledWindowClosed=true}));
    }
}
