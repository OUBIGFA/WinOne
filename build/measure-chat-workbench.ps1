param(
 [Parameter(Mandatory)][int]$OwnedProcessId,
 [Parameter(Mandatory)][string]$ProfileDirectory,
 [Parameter(Mandatory)][string]$OutputPath,
 [Parameter(Mandatory)][ValidateSet('terminal','browser','onebot')][string]$Scenario
)
$ErrorActionPreference='Stop'
$profilePath=[IO.Path]::GetFullPath($ProfileDirectory)
$fixture=Split-Path -Parent $profilePath
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $profilePath
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path ([IO.Path]::GetFullPath($OutputPath))
# The bootstrap parent may already have exited. Include the verified Host as
# an explicit root so its workers remain part of the measurement.
$ui=Get-CimInstance Win32_Process -Filter ('ProcessId = '+$OwnedProcessId)
if($ui.ExecutablePath -ne (Join-Path $fixture 'WinOne.exe')){throw 'UI does not belong to this fixture'}
$hosts=@(Get-CimInstance Win32_Process -Filter "Name = 'WinOne.Host.exe'" | Where-Object {$_.ExecutablePath -eq (Join-Path $fixture 'WinOne.Host.exe') -and $_.CommandLine.Contains($profilePath)})
if($hosts.Count -ne 1){throw 'Expected one fixture-owned Host'}
& (Join-Path $PSScriptRoot 'measure-resources.ps1') -RootProcessIds @($OwnedProcessId,$hosts[0].ProcessId) -OutputPath $OutputPath -DurationSeconds 20 -Scenario ('chat-workbench-'+$Scenario)
