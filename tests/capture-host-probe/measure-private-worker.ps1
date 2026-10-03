param([Parameter(Mandatory=$true)][string]$ReadyFile)
$ErrorActionPreference='Stop'
$ready=Get-Content -LiteralPath $ReadyFile -Raw -Encoding UTF8|ConvertFrom-Json
$process=Get-Process -Id $ready.processId
$expected=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot 'bin/Release/net10.0-windows/WinOne.NativeCapture.Probe.exe'))
if(-not [string]::Equals($process.Path,$expected,[StringComparison]::OrdinalIgnoreCase)){throw 'Not this test worker'}
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class NativeWorkerMetrics {
 [DllImport("user32.dll",SetLastError=true)] public static extern uint GetGuiResources(IntPtr process,uint flags);
}
'@
$process.Refresh()
@{
 processId=$process.Id
 privateBytes=$process.PrivateMemorySize64
 workingSetBytes=$process.WorkingSet64
 handles=$process.HandleCount
 gdi=[NativeWorkerMetrics]::GetGuiResources($process.Handle,0)
 user=[NativeWorkerMetrics]::GetGuiResources($process.Handle,1)
}|ConvertTo-Json -Compress
$process.Dispose()
