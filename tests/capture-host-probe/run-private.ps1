param(
    [ValidateSet('workflow', 'paint', 'toolbar', 'customization', 'preferences', 'scroll-progress', 'pointer', 'facade', 'native-tests', 'tray-exit', 'production-resources', 'settings-dialog', 'scroll-workflow', 'autosave', 'text-recognition', 'text-pipeline', 'latency', 'ocr-latency')][string]$Suite = 'workflow',
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$ProbeExe,
    [string]$TestFilter
)
$ErrorActionPreference = 'Stop'
if ($Suite -eq 'native-tests' -and (-not $ProbeExe -or -not $TestFilter -or $TestFilter.Contains('"'))) { throw 'Native tests require an executable and an unquoted test filter' }
if (-not $ProbeExe) { $ProbeExe = Join-Path $PSScriptRoot 'bin/Release/net10.0-windows/WinOne.NativeCapture.Probe.exe' }
$probe = [IO.Path]::GetFullPath($ProbeExe)
$output = [IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot '../../build/assert-temporary-path.ps1') -Path $output
if (-not (Test-Path -LiteralPath $probe -PathType Leaf)) { throw 'Build NativeCaptureProbe.csproj first' }
if (Test-Path -LiteralPath $output) { throw 'Choose a new output directory' }
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'probe.json')
if ($LASTEXITCODE -ne 0) { throw 'Test output must be inside a Git-ignored directory' }

# The private Win32 desktop is never switched to the user's input desktop.
# Only this invocation's child process is terminated if its deadline expires.
Add-Type -TypeDefinition @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class WinOnePrivateProbe {
 [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)] public struct Startup {
  public int size; public string reserved,desktop,title;
  public int x,y,width,height,charsX,charsY,fill,flags; public short show,reservedSize;
  public IntPtr reservedBytes,input,output,error;
 }
 [StructLayout(LayoutKind.Sequential)] public struct ProcessInfo { public IntPtr process,thread; public int pid,tid; }
 [DllImport("user32.dll",CharSet=CharSet.Unicode,SetLastError=true)] public static extern IntPtr CreateDesktopW(string name,IntPtr device,IntPtr mode,uint flags,uint access,IntPtr security);
 [DllImport("user32.dll",SetLastError=true)] public static extern bool CloseDesktop(IntPtr desktop);
 [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern bool CreateProcessW(string app,StringBuilder command,IntPtr processSecurity,IntPtr threadSecurity,bool inherit,uint flags,IntPtr environment,string cwd,ref Startup startup,out ProcessInfo process);
 [DllImport("kernel32.dll",SetLastError=true)] public static extern uint WaitForSingleObject(IntPtr handle,uint timeout);
 [DllImport("kernel32.dll",SetLastError=true)] public static extern bool GetExitCodeProcess(IntPtr process,out uint code);
 [DllImport("kernel32.dll",SetLastError=true)] public static extern bool TerminateProcess(IntPtr process,uint code);
 [DllImport("kernel32.dll")] public static extern bool CloseHandle(IntPtr handle);
 public static ProcessInfo Start(string exe,string args,string desktop,string cwd) {
  var startup=new Startup { size=Marshal.SizeOf(typeof(Startup)),desktop=desktop };
  ProcessInfo process;
  if(!CreateProcessW(exe,new StringBuilder("\""+exe+"\" "+args),IntPtr.Zero,IntPtr.Zero,false,0,IntPtr.Zero,cwd,ref startup,out process))
   throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
  return process;
 }
}
'@
$name = 'WinOne-Resource-' + [Guid]::NewGuid().ToString('N')
$desktop = [WinOnePrivateProbe]::CreateDesktopW($name, [IntPtr]::Zero, [IntPtr]::Zero, 0, 0xc7, [IntPtr]::Zero)
if ($desktop -eq [IntPtr]::Zero) { throw 'Private test desktop creation failed' }
$child = $null
try {
    $mode = switch ($Suite) { ocr-latency { '--ocr-host-latency' } latency { '--native-capture-latency' } facade { '--self-test' } autosave { '--native-autosave' } scroll-workflow { '--native-scrolling-workflow' } settings-dialog { '--settings-dialog' } production-resources { '--production-resource-suite' } tray-exit { '--tray-exit' } workflow { '--native-workflow-probe' } paint { '--native-paint-dispatch' } toolbar { '--native-toolbar-render-probe' } customization { '--native-toolbar-customization' } preferences { '--native-toolbar-preferences' } scroll-progress { '--native-scroll-progress' } pointer { '--native-pointer-dispatch' } }
    $arguments = if ($Suite -eq 'text-pipeline') { '--native-text-pipeline "'+$output+'"' } elseif ($Suite -eq 'text-recognition') { '--native-text-recognition' } elseif ($Suite -eq 'native-tests') { '-test.v -test.timeout=45s -test.run="' + $TestFilter + '"' } elseif ($Suite -in @('pointer','facade')) { $mode } else { $mode + ' "' + $output + '"' }
    $child = [WinOnePrivateProbe]::Start($probe, $arguments, $name, $root)
    $wait = [WinOnePrivateProbe]::WaitForSingleObject($child.process, $(if($Suite -in @('latency','ocr-latency')){120000}else{60000}))
    if ($wait -ne 0) {
        if (-not [WinOnePrivateProbe]::TerminateProcess($child.process, 1)) { throw 'Timed-out test could not be terminated' }
        [void][WinOnePrivateProbe]::WaitForSingleObject($child.process, 5000)
        throw ('Private test wait failed or timed out: ' + $wait)
    }
    [uint32]$code = 0
    if (-not [WinOnePrivateProbe]::GetExitCodeProcess($child.process, [ref]$code)) { throw 'Cannot read test exit status' }
    if ($code -ne 0) { throw ('Private test failed: ' + $code) }
    Write-Output "PASS: $Suite on a private desktop; output: $output"
} finally {
    if ($null -ne $child) { [void][WinOnePrivateProbe]::CloseHandle($child.thread); [void][WinOnePrivateProbe]::CloseHandle($child.process) }
    if (-not [WinOnePrivateProbe]::CloseDesktop($desktop)) { throw 'Private test desktop cleanup failed' }
}
