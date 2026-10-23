# A bounded, test-only elevated owner. RunAs does not reliably inherit the
# caller's environment. High-integrity CDP requires a component-test-debug
# binary on WebView2 150+; this owner also cleans up the fixture on exit.
param(
    [Parameter(Mandatory)][string]$OutputDirectory,
    [Parameter(Mandatory)][string]$Pipe,
    [Parameter(Mandatory)][int]$DebugPort,
    [ValidateRange(30,360)][int]$TimeoutSeconds=180
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if(-not [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){throw 'Test target owner must be elevated'}
if($Pipe -notlike '\\.\pipe\WinOne-components-test-*'){throw 'Only a fixture pipe is allowed'}
$target=$null
Add-Type -TypeDefinition @'
using System;using System.Text;using System.Collections.Generic;using System.Runtime.InteropServices;
public static class ElevatedDropWindows {
 public delegate bool EnumWindow(IntPtr window,IntPtr data);
 [DllImport("user32.dll")] static extern bool EnumChildWindows(IntPtr window,EnumWindow callback,IntPtr data);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern int GetClassNameW(IntPtr window,StringBuilder text,int count);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern IntPtr GetPropW(IntPtr window,string name);
 [DllImport("user32.dll")] static extern int GetWindowLongW(IntPtr window,int index);
 [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr window,IntPtr after,int x,int y,int width,int height,uint flags);
 [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr window,out uint process);
 public static object[] Read(IntPtr root){var rows=new List<object>();EnumWindow add=(window,data)=>{var name=new StringBuilder(128);GetClassNameW(window,name,128);uint process;var thread=GetWindowThreadProcessId(window,out process);rows.Add(new {handle=window.ToInt64(),name=name.ToString(),process=process,thread=thread,acceptFiles=(GetWindowLongW(window,-20)&16)!=0,oleTarget=GetPropW(window,"OleDropTargetInterface").ToInt64()});return true;};add(root,IntPtr.Zero);EnumChildWindows(root,add,IntPtr.Zero);return rows.ToArray();}
}
'@
try{
    $env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS="--remote-debugging-port=$DebugPort"
    $env:WINONE_DESKTOP_STARTUP_TRACE='1'
    [IO.File]::WriteAllText((Join-Path $output 'target-launch.json'),(@{ownerPid=$PID;debugArguments=$env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS;startedUTC=[DateTime]::UtcNow.ToString('o')} | ConvertTo-Json),[Text.UTF8Encoding]::new($false))
    $arguments='--pipe "'+$Pipe+'" --data-dir "'+(Join-Path $output 'profile')+'" --open-settings'
    $target=Start-Process -FilePath (Join-Path $output 'WinOne.exe') -ArgumentList $arguments -WorkingDirectory $output -PassThru
    $windowDeadline=[DateTime]::UtcNow.AddSeconds(20)
    while($target.MainWindowHandle -eq [IntPtr]::Zero){
        if($target.HasExited -or [DateTime]::UtcNow -gt $windowDeadline){throw 'Elevated target did not show its window'}
        Start-Sleep -Milliseconds 100;$target.Refresh()
    }
    # Establish z-order before publishing the PID; a delayed change could
    # otherwise cover Explorer between the guarded mouse move and mouse-down.
    [void][ElevatedDropWindows]::SetWindowPos($target.MainWindowHandle,[IntPtr]::new(-1),0,0,0,0,0x13)
    [IO.File]::WriteAllText((Join-Path $output 'target.pid'),[string]$target.Id)
    $deadline=[DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    $diagnosticAt=[DateTime]::UtcNow.AddSeconds(3)
    $handledInputs=@{}
    while(-not $target.WaitForExit(100)){
        # Only the owned test process injects input at high integrity. Explorer
        # remains the ordinary user's process and the actual OLE drag source.
        foreach($request in Get-ChildItem -LiteralPath $output -File | Where-Object {$_.Name -like '*.explorer-request.json' -or $_.Name -like '*.native-input.json'}){
            if($handledInputs.ContainsKey($request.FullName)){continue}
            $handledInputs[$request.FullName]=$true
            try{
                $inputSpec=Get-Content -LiteralPath $request.FullName -Raw -Encoding UTF8 | ConvertFrom-Json
                if($inputSpec.taskRoot -ne $output -or $inputSpec.targetPid -ne $target.Id){throw 'Input request does not belong to the elevated fixture'}
                $driver=if($request.Name -like '*.native-input.json'){'component-native-input.ps1'}else{'explorer-file-drop-probe.ps1'}
                $inputLog=& powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot $driver) -Specification $request.FullName 2>&1 | Out-String
                if($LASTEXITCODE -ne 0){throw $inputLog}
            }catch{[IO.File]::WriteAllText(($request.FullName+'.error.txt'),($_|Out-String),[Text.UTF8Encoding]::new($false))}
        }
        if($diagnosticAt -and [DateTime]::UtcNow -ge $diagnosticAt){
            # Restrict diagnostics to this fixture's executable/profile paths.
            $owned=@(Get-CimInstance Win32_Process | Where-Object {($_.ExecutablePath -and $_.ExecutablePath.StartsWith($output+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) -or ($_.Name -eq 'msedgewebview2.exe' -and $_.CommandLine.Contains($output))} | Select-Object ProcessId,ParentProcessId,Name,CommandLine)
            [IO.File]::WriteAllText((Join-Path $output 'target-processes.json'),(ConvertTo-Json -InputObject $owned -Depth 3),[Text.UTF8Encoding]::new($false))
            $target.Refresh()
            [IO.File]::WriteAllText((Join-Path $output 'target-windows.json'),(ConvertTo-Json -InputObject ([ElevatedDropWindows]::Read($target.MainWindowHandle)) -Depth 3),[Text.UTF8Encoding]::new($false))
            $diagnosticAt=$null
        }
        if((Test-Path -LiteralPath (Join-Path $output 'target.stop')) -or [DateTime]::UtcNow -gt $deadline){break}
    }
} catch {
    [IO.File]::WriteAllText((Join-Path $output 'target-error.txt'),($_ | Out-String))
    throw
} finally {
    # Scope by exact executable directory, never process name alone. This
    # owner is necessary because the normal drag source cannot kill a high-IL
    # Host left behind after a failed UI check.
    foreach($owned in @(Get-CimInstance Win32_Process | Where-Object {$_.ExecutablePath -and $_.ExecutablePath.StartsWith($output+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase) -and $_.Name -like 'WinOne*.exe'})){
        Stop-Process -Id $owned.ProcessId -ErrorAction SilentlyContinue
    }
    if($target){$target.Dispose()}
}
