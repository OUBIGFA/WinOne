[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$ContextFile,[switch]$RunSystemIntegration,[switch]$TestRollback,[switch]$TestRepair,[switch]$TestUninstallRecovery,[string]$ExportedUninstaller)
$ErrorActionPreference='Stop'
if(-not $RunSystemIntegration){throw 'Use -RunSystemIntegration to run the isolated installer and registry test'}
$contextPath=[IO.Path]::GetFullPath($ContextFile)
$testRoot=Split-Path -Parent $contextPath
$case=Get-Content -LiteralPath $contextPath -Raw -Encoding UTF8|ConvertFrom-Json
if($case.registryKey -notmatch '^Software\\WinOneInstallerTests\\[a-f0-9]{32}$'){throw 'Not an isolated installer context'}
foreach($path in @($case.installation,$case.profile,$case.shortcut)){
    if(-not ([IO.Path]::GetFullPath($path)).StartsWith($testRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Test destination escapes context directory'}
}
if(Test-Path -LiteralPath $case.installation){throw 'Installation test directory already exists'}
$old=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($case.registryKey)
if($old){$old.Close();throw 'Test registry entry already exists'}
function RunChecked([string]$executable,[string]$arguments,[int]$expectedCode=0){
    $process=Start-Process -FilePath $executable -ArgumentList $arguments -WindowStyle Hidden -PassThru
    if(-not $process.WaitForExit(120000)){throw "Installer still running: PID $($process.Id)"}
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
# Run the uninstaller outside the target, as NSIS normally does, with a
# deterministic process handle so completion is observable by the test.
$uninstaller=Join-Path $testRoot 'run-uninstall.exe'
$uninstallerSource=Join-Path $case.installation 'Uninstall.exe'
if($ExportedUninstaller){
    if((Get-FileHash -LiteralPath $ExportedUninstaller -Algorithm SHA256).Hash -ne (Get-FileHash -LiteralPath $uninstallerSource -Algorithm SHA256).Hash){throw 'Exported uninstaller differs from installed file'}
    $uninstallerSource=[IO.Path]::GetFullPath($ExportedUninstaller)
}
Copy-Item -LiteralPath $uninstallerSource -Destination $uninstaller
if($TestUninstallRecovery){
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
    RunChecked $uninstaller ('/S _?='+$case.installation)
}
$remaining=Get-ChildItem -LiteralPath $case.installation -Force
if(@($remaining).Count -ne 1 -or $remaining.Name -ne 'keep.txt'){throw 'Unexpected files remain after uninstall'}
if((Get-Content -LiteralPath $extra -Raw) -ne 'extra' -or (Get-Content -LiteralPath $sentinel -Raw) -ne 'keep'){throw 'Uninstall removed retained content'}
if(Test-Path -LiteralPath $case.shortcut){throw 'Shortcut remains after uninstall'}
$registered=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($case.registryKey)
if($registered){$registered.Close();throw 'Registration remains after uninstall'}
$result=@{installedFiles=@($manifest.files).Count; hashesVerified=$true; registrationVerified=$true; shortcutVerified=$true; uninstalled=$true; configurationPreserved=$true; extraFilePreserved=$true; rollbackVerified=[bool]$TestRollback; repairVerified=[bool]$TestRepair; uninstallRecoveryVerified=[bool]$TestUninstallRecovery; exportedUninstallerVerified=[bool]$ExportedUninstaller}
[IO.File]::WriteAllText((Join-Path $testRoot 'result.json'),($result|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
Write-Output 'Installer round trip verified'
