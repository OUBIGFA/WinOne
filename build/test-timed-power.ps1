param(
    [Parameter(Mandatory)][string]$DesktopExe,
    [Parameter(Mandatory)][string]$HostExe,
    [Parameter(Mandatory)][string]$WinTrayExe,
    [Parameter(Mandatory)][string]$PlaywrightModule,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [int]$DebugPort=19289,
    [string]$WindowShotExe=''
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if(Test-Path -LiteralPath $output){throw 'Choose a new test directory'}
$profile=Join-Path $output 'fixture'
New-Item -ItemType Directory -Path $profile -Force|Out-Null
$utf8=[Text.UTF8Encoding]::new($false)
# Fresh local profile only: no account, startup entry, program, shortcut or
# external file. Only the timer is enabled and every armed test is dry-run.
[IO.File]::WriteAllText((Join-Path $profile 'settings.json'),'{"schemaVersion":1,"theme":"light","language":"zh-CN","captureShortcut":"","pinShortcut":"","ocrShortcut":"","translateShortcut":"","selectionShortcut":"","quickAssistantShortcut":"","openShortcut":"","startup":{"runAtLogon":false,"startMinimized":true,"keepUIRunning":false}}',$utf8)
$catalog=Get-Content -LiteralPath (Join-Path $root 'internal/catalog/builtin.json') -Raw -Encoding UTF8|ConvertFrom-Json
$records=@{};foreach($id in $catalog.id){$records[$id]=@{enabled=($id -eq 'timed-power');pendingStop=$false}}
[IO.File]::WriteAllText((Join-Path $profile 'module-state.json'),(@{schemaVersion=5;revision=0;modules=$records}|ConvertTo-Json -Depth 5),$utf8)
$previous=$env:WINONE_WINDOW_SHOT_EXE
try{
    $env:WINONE_WINDOW_SHOT_EXE=$WindowShotExe
    & (Join-Path $PSScriptRoot 'test-desktop-components.ps1') -DesktopExe $DesktopExe -HostExe $HostExe -WinTrayExe $WinTrayExe -PlaywrightModule $PlaywrightModule -OutputDirectory (Join-Path $OutputDirectory 'run') -DebugPort $DebugPort -TestScript 'timed-power-native-check.mjs' -FixtureProfile $profile -DriverTimeoutSeconds 240
}finally{$env:WINONE_WINDOW_SHOT_EXE=$previous}
