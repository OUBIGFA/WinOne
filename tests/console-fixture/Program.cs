using System.Diagnostics;
using System.Globalization;
using System.Runtime.InteropServices;
using System.Text.Json;

internal static class Program
{
    [DllImport("kernel32.dll")] private static extern IntPtr GetConsoleWindow();
    [DllImport("kernel32.dll",SetLastError=true)] private static extern uint GetConsoleProcessList([Out]uint[] processes,uint count);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);

    private static int Main(string[] args)
    {
        if(args.Length!=2||!args[1].StartsWith("Local\\WinOne-ConsoleFixture-",StringComparison.Ordinal))return 2;
        var window=GetConsoleWindow();var clients=new uint[16];var count=GetConsoleProcessList(clients,(uint)clients.Length);
        // Never expose or close a console shared with the user or another test.
        if(window==IntPtr.Zero||count!=1||clients[0]!=(uint)Environment.ProcessId)throw new InvalidOperationException("A separate console is required");
        GetWindowThreadProcessId(window,out var consoleOwner);
        using var exit=new EventWaitHandle(false,EventResetMode.ManualReset,args[1]);
        Console.Title="WinOne owned console fixture";
        Console.WriteLine("WinOne lifecycle test — generated content only");
        File.WriteAllText(args[0],JsonSerializer.Serialize(new{handle=window.ToInt64().ToString(CultureInfo.InvariantCulture),pid=Environment.ProcessId,created=Process.GetCurrentProcess().StartTime.ToUniversalTime().ToFileTimeUtc().ToString(CultureInfo.InvariantCulture),consoleOwner,consoleClients=count,inputRedirected=Console.IsInputRedirected,outputRedirected=Console.IsOutputRedirected}));
        return exit.WaitOne(TimeSpan.FromMinutes(3))?0:3;
    }
}
