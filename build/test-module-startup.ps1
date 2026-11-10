param(
    [Parameter(Mandatory)][string]$BundleDirectory,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$GoExe='go',
    [switch]$RunSystemChecks,
    [switch]$ForegroundHotkey,
    [switch]$PrivateChild
)
$ErrorActionPreference='Stop'
if(-not $RunSystemChecks){throw 'Use -RunSystemChecks for owned-window startup and restoration checks'}
$root=Split-Path -Parent $PSScriptRoot
$bundle=(Resolve-Path -LiteralPath $BundleDirectory).Path
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$GoExe=(Get-Command $GoExe -ErrorAction Stop).Source
if(Test-Path -LiteralPath $output){throw 'Choose a new startup check directory'}
. (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
if(-not $PrivateChild){
    $desktop=[WinOnePrivateTestDesktop]::new([bool]$ForegroundHotkey);$child=$null
    $before=[WinOnePrivateTestDesktop]::InputName()
    try{
        $args='-NoProfile -File "'+$PSCommandPath+'" -RunSystemChecks -PrivateChild -BundleDirectory "'+$bundle+'" -OutputDirectory "'+$output+'" -GoExe "'+$GoExe+'"'
        if($ForegroundHotkey){$args+=' -ForegroundHotkey'}
        $child=$desktop.Start((Get-Command powershell.exe).Source,$args,$root)
        if($ForegroundHotkey){
            $ready=Join-Path $output 'ready.json';$deadline=[DateTime]::UtcNow.AddSeconds(90)
            while(-not(Test-Path -LiteralPath $ready)){
                if($child.HasExited -or [DateTime]::UtcNow -gt $deadline){throw 'Foreground fixture preparation failed'}
                Start-Sleep -Milliseconds 100
            }
            $desktop.BeginForegroundTest(15)
            [IO.File]::WriteAllText((Join-Path $output 'input-active'),'1',[Text.UTF8Encoding]::new($false))
            while(-not $child.WaitForExit(100)){
                if(Test-Path -LiteralPath (Join-Path $output 'input-finished')){$null=$desktop.RestoreInputDesktop()}
            }
        }
        if(-not $child.WaitForExit(120000)){throw 'Private startup checks timed out'}
        if(Test-Path -LiteralPath (Join-Path $output 'tests.log')){Get-Content -LiteralPath (Join-Path $output 'tests.log') -Encoding UTF8}
        if($child.ExitCode -ne 0){throw 'Private startup checks failed'}
    }finally{
        $restored=$desktop.RestoreInputDesktop()
        if($child){if(-not $child.HasExited){$child.Kill();$child.WaitForExit()};$child.Dispose()}
        $desktop.Dispose()
        if(Test-Path -LiteralPath $output){
            $record=@{before=$before;after=[WinOnePrivateTestDesktop]::InputName();restored=$restored}
            [IO.File]::WriteAllText((Join-Path $output 'desktop.json'),($record|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
        }
    }
    return
}
if(-not [WinOnePrivateTestDesktop]::CurrentName().StartsWith('WinOne-Test-')){throw 'Owned private desktop required'}
New-Item -ItemType Directory -Path $output | Out-Null
$env:TEMP=$output;$env:TMP=$output
$env:WINONE_AOT_EXE=Join-Path $bundle 'always-on-top/WinOne.AlwaysOnTop.exe'
$env:WINONE_WINDOW_FIXTURE=Join-Path $root 'tests/window-fixture/bin/Release/net10.0-windows/WinOne.WindowFixture.exe'
$env:WINONE_MODULE_STARTUP_TEST='1'
Push-Location $root
try {
    & dotnet build tests/window-fixture/WindowFixture.csproj -c Release --nologo -v:q *> (Join-Path $output 'fixture.log')
    if($LASTEXITCODE -ne 0){throw 'Window fixture build failed'}
    $test='^TestAlwaysOnTop(FirstActionLatency|StateRestorationWithoutFrame)$'
    if($ForegroundHotkey){
        $env:WINONE_PIN_HOTKEY_OUTPUT=$output
        $env:WINONE_PIN_HOTKEY_BUNDLE=$bundle
        $env:WINONE_DESKTOP_CLI=(Get-Command agent-desktop -ErrorAction Stop).Source
        $test='^TestHostPinShortcutFirstPress$'
    }
    & $GoExe test ./tests/integration -run $test -count=1 -v -timeout=90s *> (Join-Path $output 'tests.log')
    if($LASTEXITCODE -ne 0){throw 'Native startup checks failed'}
}finally{Pop-Location}
