[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$ContextFile,[switch]$RunSystemIntegration,[switch]$TestRollback,[switch]$TestRepair,[switch]$TestOverwrite,[switch]$TestUninstallRecovery,[string]$ExportedUninstaller,[ValidateRange(30,600)][int]$OperationTimeoutSeconds=120)
$ErrorActionPreference='Stop'
if(-not $RunSystemIntegration){throw 'Use -RunSystemIntegration to run the isolated installer and registry test'}
$contextPath=[IO.Path]::GetFullPath($ContextFile)
$testRoot=Split-Path -Parent $contextPath
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $testRoot
$case=Get-Content -LiteralPath $contextPath -Raw -Encoding UTF8|ConvertFrom-Json
if($case.registryKey -notmatch '^Software\\WinOneInstallerTests\\[a-f0-9]{32}$'){throw 'Not an isolated installer context'}
foreach($path in @($case.installation,$case.profile,$case.shortcut)){
    if(-not ([IO.Path]::GetFullPath($path)).StartsWith($testRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Test destination escapes context directory'}
}
if(Test-Path -LiteralPath $case.installation){throw 'Installation test directory already exists'}
$old=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($case.registryKey)
if($old){$old.Close();throw 'Test registry entry already exists'}
$operationTimings=[Collections.Generic.List[object]]::new()
function RunChecked([string]$executable,[string]$arguments,[int]$expectedCode=0){
    $elapsed=[Diagnostics.Stopwatch]::StartNew()
    $process=Start-Process -FilePath $executable -ArgumentList $arguments -WindowStyle Hidden -PassThru
    # Retain the process handle before waiting so short-lived installers still
    # expose their actual exit code through Windows PowerShell.
    $null=$process.Handle
    # This is a runner deadline, not an installer performance target. Full OCR
    # payloads may need a larger explicit budget; keep their actual timings.
    if(-not $process.WaitForExit($OperationTimeoutSeconds*1000)){throw "Installer still running after $OperationTimeoutSeconds seconds: PID $($process.Id)"}
    $operationTimings.Add([ordered]@{executable=[IO.Path]::GetFileName($executable);arguments=$arguments;seconds=[Math]::Round($elapsed.Elapsed.TotalSeconds,3);exitCode=$process.ExitCode;expectedCode=$expectedCode})
    Write-Output ("Installer operation: {0}; exit {1}; {2:N3} seconds" -f [IO.Path]::GetFileName($executable),$process.ExitCode,$elapsed.Elapsed.TotalSeconds)
    if($process.ExitCode -ne $expectedCode){throw "Installer exit code $($process.ExitCode), expected $expectedCode"}
}
[IO.Directory]::CreateDirectory($case.profile)|Out-Null
$sentinel=Join-Path $case.profile 'keep.txt'
[IO.File]::WriteAllText($sentinel,'keep',[Text.UTF8Encoding]::new($false))
if($TestRollback){
    foreach($point in @(@{name='after-files';code=21},@{name='after-registration';code=22})){
        RunChecked $case.installer ('/S /FAILPOINT='+$point.name+' /D='+$case.installation) $point.code
        if((Test-Path -LiteralPath $case.installation) -and @(Get-ChildItem -LiteralPath $case.installation -Force).Count -gt 0){throw "Rollback retained installation content: $($point.name)"}
        if(Test-Path -LiteralPath $case.shortcut){throw 'Rollback retained shortcut'}
        $entry=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($case.registryKey)
        if($entry){$entry.Close();throw 'Rollback retained registration'}
        if((Get-Content -LiteralPath $sentinel -Raw) -ne 'keep'){throw 'Rollback changed configuration'}
    }
}
if($TestRepair){
    RunChecked $case.installer ('/S /FAILPOINT=interrupt-after-files /D='+$case.installation) 31
    $pending=Join-Path $case.installation '.winone-pending'
    $record=Join-Path $case.installation '.winone-recovery.ini'
    if(-not (Test-Path -LiteralPath $pending) -or -not (Test-Path -LiteralPath $record)){throw 'Interrupted installation lost recovery data'}
    RunChecked (Join-Path $case.installation 'WinOne.Host.exe') ('--headless --data-dir "'+$case.profile+'"') 1
    $program=Join-Path $case.installation 'WinOne.exe'
    [IO.File]::WriteAllText($program,'interrupted payload',[Text.UTF8Encoding]::new($false))
    $extra=Join-Path $case.installation 'keep.txt'
    [IO.File]::WriteAllText($extra,'extra',[Text.UTF8Encoding]::new($false))
    $original=[IO.File]::ReadAllText($record,[Text.Encoding]::Unicode)
    $mismatch=[regex]::Replace($original,'PackageHash=[^\r\n]+','PackageHash=mismatch')
    [IO.File]::WriteAllText($record,$mismatch,[Text.Encoding]::Unicode)
    RunChecked $case.installer ('/S /REPAIR /D='+$case.installation) 3
    if([IO.File]::ReadAllText($program) -ne 'interrupted payload'){throw 'Mismatched repair changed payload'}
    [IO.File]::WriteAllText($record,$original,[Text.Encoding]::Unicode)
    RunChecked $case.installer ('/S /REPAIR /FAILPOINT=after-files /D='+$case.installation) 2
    if(-not (Test-Path -LiteralPath $pending) -or [IO.File]::ReadAllText($extra) -ne 'extra'){throw 'Failed repair discarded recovery data or extras'}
    RunChecked $case.installer ('/S /REPAIR /D='+$case.installation)
    if((Test-Path -LiteralPath $pending) -or (Test-Path -LiteralPath $record)){throw 'Successful repair retained incomplete state'}
    if([IO.File]::ReadAllText($extra) -ne 'extra'){throw 'Repair removed extra file'}
}else{
    RunChecked $case.installer ('/S /D='+$case.installation)
}
$manifest=Get-Content -LiteralPath (Join-Path $case.installation 'manifest.json') -Raw -Encoding UTF8|ConvertFrom-Json
foreach($file in $manifest.files){
    $path=Join-Path $case.installation $file.path
    if((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $file.sha256){throw "Installed hash mismatch: $($file.path)"}
}
$registered=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($case.registryKey)
if(-not $registered){throw 'Installer registration missing'}
try{if($registered.GetValue('InstallLocation') -ne $case.installation){throw 'Incorrect registered location'}}finally{$registered.Close()}
$shell=New-Object -ComObject WScript.Shell
$shortcut=$shell.CreateShortcut($case.shortcut)
if($shortcut.TargetPath -ne (Join-Path $case.installation 'WinOne.exe')){throw 'Incorrect shortcut target'}
[void][Runtime.InteropServices.Marshal]::ReleaseComObject($shortcut)
[void][Runtime.InteropServices.Marshal]::ReleaseComObject($shell)
$extra=Join-Path $case.installation 'keep.txt'
[IO.File]::WriteAllText($extra,'extra',[Text.UTF8Encoding]::new($false))
if($TestOverwrite){
    # A registered copy must take the update transaction, not the first-install
    # path. Keep an unrelated file and the user profile, then require the full
    # payload, registration and shortcut to match again.
    RunChecked $case.installer ('/S /D='+$case.installation)
    $overwritten=Get-Content -LiteralPath (Join-Path $case.installation 'manifest.json') -Raw -Encoding UTF8|ConvertFrom-Json
    foreach($file in $overwritten.files){
        $path=Join-Path $case.installation $file.path
        if((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $file.sha256){throw "Overwrite hash mismatch: $($file.path)"}
    }
    if([IO.File]::ReadAllText($extra) -ne 'extra'){throw 'Overwrite removed an unrelated file'}
    if((Get-Content -LiteralPath $sentinel -Raw) -ne 'keep'){throw 'Overwrite changed configuration'}
    $entry=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($case.registryKey)
    if(-not $entry){throw 'Overwrite removed registration'}
    try{if($entry.GetValue('InstallLocation') -ne $case.installation){throw 'Overwrite changed registered location'}}finally{$entry.Close()}
}
# Run the uninstaller outside the target, as NSIS normally does, with a
# deterministic process handle so completion is observable by the test.
$uninstaller=Join-Path $testRoot 'run-uninstall.exe'
$uninstallerSource=Join-Path $case.installation 'Uninstall.exe'
if($ExportedUninstaller){
    if((Get-FileHash -LiteralPath $ExportedUninstaller -Algorithm SHA256).Hash -ne (Get-FileHash -LiteralPath $uninstallerSource -Algorithm SHA256).Hash){throw 'Exported uninstaller differs from installed file'}
    $uninstallerSource=[IO.Path]::GetFullPath($ExportedUninstaller)
}
if($TestUninstallRecovery){
    Copy-Item -LiteralPath $uninstallerSource -Destination $uninstaller
    RunChecked $uninstaller ('/S /FAILPOINT=interrupt-after-removal _?='+$case.installation) 32
    foreach($name in @('WinOne.Host.exe','WinOne.WinTray.exe')){if(Test-Path -LiteralPath (Join-Path $case.installation $name)){throw 'Uninstall interruption did not remove bootstrap files'}}
    $pending=Join-Path $case.installation '.winone-pending'
    $record=Join-Path $case.installation '.winone-recovery.ini'
    if(-not (Test-Path -LiteralPath $pending) -or -not (Test-Path -LiteralPath $record)){throw 'Uninstall recovery evidence missing'}
    $original=[IO.File]::ReadAllText($record,[Text.Encoding]::Unicode)
    [IO.File]::WriteAllText($record,[regex]::Replace($original,'PackageHash=[^\r\n]+','PackageHash=mismatch'),[Text.Encoding]::Unicode)
    RunChecked $case.installer ('/S /FINISHUNINSTALL /D='+$case.installation) 3
    if(-not (Test-Path -LiteralPath $pending) -or -not (Test-Path -LiteralPath $case.shortcut)){throw 'Mismatched uninstall recovery changed state'}
    [IO.File]::WriteAllText($record,$original,[Text.Encoding]::Unicode)
    RunChecked $case.installer ('/S /FINISHUNINSTALL /FAILPOINT=interrupt-after-registration-removal /D='+$case.installation) 33
    $entry=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($case.registryKey)
    if($entry){$entry.Close();throw 'Second interruption did not remove registration'}
    if(-not (Test-Path -LiteralPath $pending)){throw 'Second interruption lost recovery marker'}
    RunChecked $case.installer ('/S /FINISHUNINSTALL /D='+$case.installation)
}else{
    Copy-Item -LiteralPath $uninstallerSource -Destination $uninstaller
    RunChecked $uninstaller ('/S _?='+$case.installation)
}
# In-place NSIS uninstall launches a temporary child and the registered
# executable returns before that child finishes. Observe the retained-content
# postcondition within the same bounded operation budget, not the parent exit.
$uninstallDeadline=[DateTime]::UtcNow.AddSeconds($OperationTimeoutSeconds)
do {
    $remaining=@(Get-ChildItem -LiteralPath $case.installation -Force)
    $removalComplete=$remaining.Count -eq 1 -and $remaining[0].Name -eq 'keep.txt' -and -not(Test-Path -LiteralPath $case.shortcut)
    if($removalComplete){break}
    Start-Sleep -Milliseconds 200
} while([DateTime]::UtcNow -lt $uninstallDeadline)
if(-not $removalComplete){throw 'Unexpected files or shortcut remain after uninstall'}
if((Get-Content -LiteralPath $extra -Raw) -ne 'extra' -or (Get-Content -LiteralPath $sentinel -Raw) -ne 'keep'){throw 'Uninstall removed retained content'}
if(Test-Path -LiteralPath $case.shortcut){throw 'Shortcut remains after uninstall'}
$registered=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($case.registryKey)
if($registered){$registered.Close();throw 'Registration remains after uninstall'}
$result=@{installedFiles=@($manifest.files).Count; hashesVerified=$true; registrationVerified=$true; shortcutVerified=$true; uninstalled=$true; configurationPreserved=$true; extraFilePreserved=$true; rollbackVerified=[bool]$TestRollback; repairVerified=[bool]$TestRepair; uninstallRecoveryVerified=[bool]$TestUninstallRecovery; exportedUninstallerVerified=[bool]$ExportedUninstaller; overwriteVerified=[bool]$TestOverwrite}
$result.operationTimeoutSeconds=$OperationTimeoutSeconds
$result.operationTimings=$operationTimings.ToArray()
[IO.File]::WriteAllText((Join-Path $testRoot 'result.json'),($result|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
Write-Output 'Installer round trip verified'
