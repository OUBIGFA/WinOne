using System.ComponentModel;
using System.Runtime.InteropServices;

namespace WinOne.Capture;

// Windows can classify a hidden worker as background work even while the
// visible editor is waiting for it. Request HighQoS only for the active action,
// then restore the complete prior policy, including timer-resolution flags.
// https://learn.microsoft.com/windows/win32/procthread/quality-of-service
internal sealed class InteractiveWorkScope : IDisposable
{
    [StructLayout(LayoutKind.Sequential)]
    internal struct PowerState { public uint Version, ControlMask, StateMask; }
    private const int ProcessPowerThrottling = 4;
    private const uint ExecutionSpeed = 1;
    private readonly PowerState original;
    private bool disposed;

    [DllImport("kernel32.dll")]
    private static extern IntPtr GetCurrentProcess();
    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GetProcessInformation(IntPtr process, int informationClass, ref PowerState information, uint size);
    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetProcessInformation(IntPtr process, int informationClass, ref PowerState information, uint size);

    internal static PowerState ReadState()
    {
        var state = new PowerState { Version = 1 };
        if (!GetProcessInformation(GetCurrentProcess(), ProcessPowerThrottling, ref state, (uint)Marshal.SizeOf<PowerState>()))
            throw new Win32Exception(Marshal.GetLastWin32Error(), "无法读取截图进程调度设置");
        return state;
    }

    public InteractiveWorkScope()
    {
        original = ReadState();
        var active = original;
        active.ControlMask |= ExecutionSpeed;
        active.StateMask &= ~ExecutionSpeed;
        Apply(active);
    }

    private static void Apply(PowerState state)
    {
        if (!SetProcessInformation(GetCurrentProcess(), ProcessPowerThrottling, ref state, (uint)Marshal.SizeOf<PowerState>()))
            throw new Win32Exception(Marshal.GetLastWin32Error(), "无法更新截图进程调度设置");
    }

    public void Dispose()
    {
        if (disposed) return;
        Apply(original);
        disposed = true;
    }
}
