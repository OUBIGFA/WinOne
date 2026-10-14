using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;
using System.Windows.Automation;

// Operates only the file dialog of this invocation's known WinOne process on
// its private test desktop. It never enumerates or types into user dialogs.
internal static class ModuleFileDialogProbe
{
    private delegate bool EnumProc(IntPtr window,IntPtr state);
    [DllImport("user32.dll")] private static extern bool EnumWindows(EnumProc callback,IntPtr state);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr window,out int pid);
    [DllImport("user32.dll",CharSet=CharSet.Unicode)] private static extern int GetClassNameW(IntPtr window,StringBuilder name,int length);
    [DllImport("user32.dll")] private static extern bool IsWindow(IntPtr window);
    [DllImport("user32.dll")] private static extern bool IsChild(IntPtr parent,IntPtr child);
    [DllImport("user32.dll",SetLastError=true)] private static extern bool PostMessageW(IntPtr window,uint message,IntPtr wParam,IntPtr lParam);

    // A dialog HWND is published before its controls finish initialization.
    // Wait for the actionable control rather than treating HWND discovery as ready.
    private static AutomationElement ReadyButton(AutomationElement root,string id,Stopwatch deadline)
    {
        while(deadline.Elapsed<TimeSpan.FromSeconds(10)){
            var button=root.FindFirst(TreeScope.Descendants,new AndCondition(new PropertyCondition(AutomationElement.AutomationIdProperty,id),new PropertyCondition(AutomationElement.ControlTypeProperty,ControlType.Button)));
            if(button is not null&&button.Current.IsEnabled&&!button.Current.IsOffscreen&&button.Current.NativeWindowHandle!=0)return button;
            Thread.Sleep(25);
        }
        throw new Exception($"Dialog button {id} did not become actionable");
    }

    // The .NET WindowsButton UIA proxy calls Misc.SetFocus before BM_CLICK,
    // which fails on a non-input desktop even when the button is enabled.
    // Deliver the same native button action on this owned test desktop. The
    // caller still verifies that the real dialog closes and the app responds.
    // https://github.com/dotnet/wpf/blob/main/src/Microsoft.DotNet.Wpf/src/UIAutomation/UIAutomationClientSideProviders/MS/Internal/AutomationProxies/WindowsButton.cs
    private static void ClickButton(AutomationElement button,IntPtr dialog,int pid)
    {
        var window=new IntPtr(button.Current.NativeWindowHandle);
        GetWindowThreadProcessId(window,out var owner);
        if(owner!=pid||!IsChild(dialog,window))throw new Exception("Button does not belong to the owned file dialog");
        if(!PostMessageW(window,0x00F5,IntPtr.Zero,IntPtr.Zero))throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
    }

    internal static int Run(int pid,string file)
    {
        if(!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Components-",StringComparison.Ordinal))throw new InvalidOperationException("Use the isolated component desktop");
        if(pid<=0||(file!="-"&&(!Path.IsPathFullyQualified(file)||!File.Exists(file))))throw new InvalidOperationException("Invalid owned dialog target");
        var deadline=Stopwatch.StartNew();IntPtr dialog=IntPtr.Zero;
        while(dialog==IntPtr.Zero&&deadline.Elapsed<TimeSpan.FromSeconds(10)){
            EnumWindows((window,_)=>{GetWindowThreadProcessId(window,out var owner);if(owner!=pid)return true;var name=new StringBuilder(80);GetClassNameW(window,name,name.Capacity);if(name.ToString()=="#32770"){dialog=window;return false;}return true;},IntPtr.Zero);
            if(dialog==IntPtr.Zero)Thread.Sleep(25);
        }
        if(dialog==IntPtr.Zero)throw new Exception("Owned module file dialog did not open");
        var root=AutomationElement.FromHandle(dialog);
        if(file=="-"){
            var cancel=ReadyButton(root,"2",deadline);
            ClickButton(cancel,dialog,pid);
            while(IsWindow(dialog)&&deadline.Elapsed<TimeSpan.FromSeconds(15))Thread.Sleep(25);
            if(IsWindow(dialog))throw new Exception("Owned file dialog did not cancel");return 0;
        }
        var open=ReadyButton(root,"1",deadline);
        var input=root.FindFirst(TreeScope.Descendants,new PropertyCondition(AutomationElement.AutomationIdProperty,"1148"));
        if(input is null)throw new Exception("File-name control missing");
        if(!input.TryGetCurrentPattern(ValuePattern.Pattern,out var value)){
            input=input.FindFirst(TreeScope.Descendants,new PropertyCondition(AutomationElement.ControlTypeProperty,ControlType.Edit));
            if(input is null||!input.TryGetCurrentPattern(ValuePattern.Pattern,out value))throw new Exception("File-name value is unavailable");
        }
        ((ValuePattern)value).SetValue(file);
        ClickButton(open,dialog,pid);
        while(IsWindow(dialog)&&deadline.Elapsed<TimeSpan.FromSeconds(15))Thread.Sleep(25);
        if(IsWindow(dialog))throw new Exception("Owned file dialog did not accept selection");
        return 0;
    }
}
