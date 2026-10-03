Set-StrictMode -Version Latest

function Get-TrackedProcessFamily {
    param([object[]]$Inventory, [hashtable]$Known)
    $current = @{}
    $owned = @{}
    $problems = [System.Collections.Generic.List[string]]::new()
    foreach ($process in $Inventory) { $current[[string]$process.ProcessId] = $process }
    foreach ($key in @($Known.Keys)) {
        if ($current.ContainsKey($key)) {
            $process = $current[$key]
            if ($null -ne $process.CreationDate -and [string]$process.CreationDate.ToUniversalTime().Ticks -eq $Known[$key]) { $owned[$key] = $process }
        }
    }
    do {
        $changed = $false
        foreach ($process in $Inventory) {
            $key = [string]$process.ProcessId
            $parentKey = [string]$process.ParentProcessId
            if ($owned.ContainsKey($key) -or -not $owned.ContainsKey($parentKey)) { continue }
            if ($null -eq $process.CreationDate) { $problems.Add("Cannot identify child process $key"); continue }
            if ($process.CreationDate -lt $owned[$parentKey].CreationDate) { continue }
            $owned[$key] = $process
            $Known[$key] = [string]$process.CreationDate.ToUniversalTime().Ticks
            $changed = $true
        }
    } while ($changed)
    [pscustomobject]@{ Members = @($owned.Values | Sort-Object ProcessId); Errors = @($problems | Select-Object -Unique) }
}

function Initialize-ResourceNative {
    if ('WinOneResourceProbe.Native' -as [type]) { return }
    Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Diagnostics;
using System.Runtime.InteropServices;
namespace WinOneResourceProbe {
    public sealed class Stats { public long Cpu100Ns; public long Timestamp; public uint Gdi; public uint User; }
    public sealed class WindowStats { public int ClientWidth; public int ClientHeight; public uint Dpi; public bool Foreground; public bool Minimized; public bool Visible; public bool HasCaption; public ulong AvailablePhysicalBytes; }
    public static class Native {
        [StructLayout(LayoutKind.Sequential)] struct RECT { public int Left,Top,Right,Bottom; }
        [StructLayout(LayoutKind.Sequential)] struct MEMORY { public uint Length,Load; public ulong TotalPhys,AvailPhys,TotalPage,AvailPage,TotalVirtual,AvailVirtual,AvailExtendedVirtual; }
        [DllImport("user32.dll", SetLastError=true)] static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);
        [DllImport("user32.dll", SetLastError=true)] static extern bool GetClientRect(IntPtr window,out RECT rectangle);
        [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
        [DllImport("user32.dll")] static extern uint GetDpiForWindow(IntPtr window);
        [DllImport("user32.dll")] static extern bool IsIconic(IntPtr window);
        [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr window);
        [DllImport("user32.dll", EntryPoint="GetWindowLongPtrW")] static extern IntPtr GetWindowLongPtr(IntPtr window,int index);
        [DllImport("kernel32.dll", SetLastError=true)] static extern bool GlobalMemoryStatusEx(ref MEMORY memory);
        public static WindowStats ReadWindow(IntPtr window,uint expectedPid) {
            uint pid;
            if (window==IntPtr.Zero || expectedPid==0 || GetWindowThreadProcessId(window,out pid)==0 || pid!=expectedPid) throw new InvalidOperationException("Window identity changed");
            RECT rectangle;
            if (!GetClientRect(window,out rectangle)) throw new Win32Exception(Marshal.GetLastWin32Error());
            MEMORY memory=new MEMORY { Length=(uint)Marshal.SizeOf(typeof(MEMORY)) };
            if (!GlobalMemoryStatusEx(ref memory)) throw new Win32Exception(Marshal.GetLastWin32Error());
            uint dpi=GetDpiForWindow(window);
            if (dpi==0) throw new InvalidOperationException("Window DPI unavailable");
            return new WindowStats { ClientWidth=rectangle.Right-rectangle.Left,ClientHeight=rectangle.Bottom-rectangle.Top,Dpi=dpi,Foreground=GetForegroundWindow()==window,Minimized=IsIconic(window),Visible=IsWindowVisible(window),HasCaption=(GetWindowLongPtr(window,-16).ToInt64() & 0x00C00000)!=0,AvailablePhysicalBytes=memory.AvailPhys };
        }
        [StructLayout(LayoutKind.Sequential)] struct FT { public uint Low; public uint High; public long Value { get { return ((long)High << 32) | Low; } } }
        [DllImport("kernel32.dll", SetLastError=true)] static extern IntPtr OpenProcess(uint access, bool inherit, uint pid);
        [DllImport("kernel32.dll", SetLastError=true)] static extern bool CloseHandle(IntPtr handle);
        [DllImport("kernel32.dll", SetLastError=true)] static extern bool GetProcessTimes(IntPtr handle, out FT created, out FT exited, out FT kernel, out FT user);
        [DllImport("kernel32.dll")] static extern void SetLastError(uint code);
        [DllImport("user32.dll", SetLastError=true)] static extern uint GetGuiResources(IntPtr handle, uint flag);
        static uint Gui(IntPtr handle, uint flag) {
            SetLastError(0);
            uint value=GetGuiResources(handle,flag);
            int error=Marshal.GetLastWin32Error();
            if (value==0 && error!=0) throw new Win32Exception(error);
            return value;
        }
        public static Stats Read(uint pid, long expectedCreatedTicks) {
            IntPtr handle=OpenProcess(0x1000,false,pid);
            if (handle==IntPtr.Zero) throw new Win32Exception(Marshal.GetLastWin32Error());
            try {
                FT created,exited,kernel,user;
                if (!GetProcessTimes(handle,out created,out exited,out kernel,out user)) throw new Win32Exception(Marshal.GetLastWin32Error());
                if (Math.Abs(DateTime.FromFileTimeUtc(created.Value).Ticks-expectedCreatedTicks)>TimeSpan.TicksPerMillisecond) throw new InvalidOperationException("Process identity changed");
                return new Stats { Cpu100Ns=kernel.Value+user.Value, Timestamp=Stopwatch.GetTimestamp(), Gdi=Gui(handle,0), User=Gui(handle,1) };
            } finally { CloseHandle(handle); }
        }
    }
}
'@
}

function Get-ProcessResourceRows {
    param([object]$Family, [hashtable]$Counters, [hashtable]$Previous, [int]$LogicalProcessors)
    if ($LogicalProcessors -lt 1) { throw 'Logical processor count is invalid' }
    Initialize-ResourceNative
    $rows = [System.Collections.Generic.List[object]]::new()
    $problems = [System.Collections.Generic.List[string]]::new()
    foreach ($problem in $Family.Errors) { $problems.Add($problem) }
    foreach ($process in $Family.Members) {
        $key = [string]$process.ProcessId
        try {
            if (-not $Counters.ContainsKey($key)) { throw 'Performance counter is not available' }
            $counter = $Counters[$key]
            foreach ($property in @('WorkingSetPrivate','PrivateBytes','HandleCount')) {
                if ($null -eq $counter.$property) { throw "Missing counter: $property" }
            }
            $stamp = $process.CreationDate.ToUniversalTime().Ticks
            $native = [WinOneResourceProbe.Native]::Read([uint32]$process.ProcessId, $stamp)
            $identity = "${key}:$stamp"
            $cpu = $null
            if ($Previous.ContainsKey($identity)) {
                $prior = $Previous[$identity]
                $elapsed = ($native.Timestamp - $prior.Timestamp) / [double][System.Diagnostics.Stopwatch]::Frequency
                $delta = $native.Cpu100Ns - $prior.Cpu100Ns
                if ($elapsed -le 0 -or $delta -lt 0) { throw 'CPU counters moved backwards' }
                $cpu = 100.0 * $delta / (10000000.0 * $elapsed * $LogicalProcessors)
            }
            $Previous[$identity] = $native
            $rows.Add([pscustomobject]@{
                ProcessId=[uint32]$process.ProcessId; ParentProcessId=[uint32]$process.ParentProcessId; Name=$process.Name; CreatedUtc=$process.CreationDate.ToUniversalTime().ToString('o')
                PrivateWorkingSetBytes=[long]$counter.WorkingSetPrivate; PrivateBytes=[long]$counter.PrivateBytes
                CpuPercent=$cpu; Handles=[long]$counter.HandleCount; GdiObjects=$native.Gdi; UserObjects=$native.User
            })
        } catch { $problems.Add("Process ${key}: $($_.Exception.Message)") }
    }
    $all = @($rows)
    $totalCpu = $null
    if ($all.Count -gt 0 -and @($all | Where-Object { $null -eq $_.CpuPercent }).Count -eq 0 -and $problems.Count -eq 0) {
        $totalCpu = ($all | Measure-Object CpuPercent -Sum).Sum
    }
    [pscustomobject]@{
        Complete=($problems.Count -eq 0); Processes=$all; Errors=@($problems)
        Totals=[pscustomobject]@{
            ProcessCount=$all.Count
            PrivateWorkingSetBytes=($all | Measure-Object PrivateWorkingSetBytes -Sum).Sum
            PrivateBytes=($all | Measure-Object PrivateBytes -Sum).Sum
            CpuPercent=$totalCpu; Handles=($all | Measure-Object Handles -Sum).Sum
            GdiObjects=($all | Measure-Object GdiObjects -Sum).Sum; UserObjects=($all | Measure-Object UserObjects -Sum).Sum
        }
    }
}

function Get-WindowResourceContext {
    param([long]$WindowHandle,[uint32]$ProcessId)
    Initialize-ResourceNative
    [WinOneResourceProbe.Native]::ReadWindow([IntPtr]$WindowHandle,$ProcessId)
}

function Test-HostOnlyResourceReport {
    param([object]$Report,[uint32]$HostProcessId)
    if ($null -eq $Report -or $null -eq $Report.Samples -or @($Report.Samples).Count -eq 0) { return $false }
    foreach ($sample in $Report.Samples) {
        $processes=@($sample.Processes)
        if (-not $sample.Complete -or $processes.Count -ne 1) { return $false }
        if ($processes[0].ProcessId -ne $HostProcessId -or $processes[0].Name -ne 'WinOne.Host.exe') { return $false }
    }
    return $true
}

function Test-OpenWindowResourceReport {
    param([object]$Report,[uint32]$HostProcessId,[uint32]$DesktopProcessId)
    if ($HostProcessId -eq 0 -or $DesktopProcessId -eq 0 -or $HostProcessId -eq $DesktopProcessId) { return $false }
    if ($null -eq $Report -or $null -eq $Report.PSObject.Properties['Samples'] -or $null -eq $Report.Samples -or @($Report.Samples).Count -eq 0) { return $false }
    foreach ($sample in $Report.Samples) {
        if ($null -eq $sample) { return $false }
        foreach ($property in @('Complete','Processes','Window')) { if ($null -eq $sample.PSObject.Properties[$property]) { return $false } }
        if ($sample.Complete -isnot [bool] -or -not $sample.Complete -or $null -eq $sample.Window) { return $false }
        $window=$sample.Window
        foreach ($property in @('Visible','Minimized','ClientWidth','ClientHeight','Dpi')) { if ($null -eq $window.PSObject.Properties[$property]) { return $false } }
        if ($window.Visible -isnot [bool] -or -not $window.Visible -or $window.Minimized -isnot [bool] -or $window.Minimized) { return $false }
        foreach ($property in @('ClientWidth','ClientHeight','Dpi')) {
            $value=$window.$property
            if (($value -isnot [int] -and $value -isnot [long] -and $value -isnot [uint32] -and $value -isnot [uint64]) -or $value -le 0) { return $false }
        }
        $processes=@($sample.Processes)
        foreach ($process in $processes) {
            if ($null -eq $process -or $null -eq $process.PSObject.Properties['ProcessId'] -or $null -eq $process.PSObject.Properties['Name']) { return $false }
        }
        if (@($processes | Where-Object { $_.ProcessId -eq $HostProcessId -and $_.Name -eq 'WinOne.Host.exe' }).Count -ne 1) { return $false }
        if (@($processes | Where-Object { $_.ProcessId -eq $DesktopProcessId -and $_.Name -eq 'WinOne.exe' }).Count -ne 1) { return $false }
        if (@($processes | Where-Object Name -eq 'msedgewebview2.exe').Count -eq 0) { return $false }
    }
    return $true
}

Export-ModuleMember -Function Get-TrackedProcessFamily,Get-ProcessResourceRows,Get-WindowResourceContext,Test-HostOnlyResourceReport,Test-OpenWindowResourceReport
