param(
    [Parameter(Mandatory)][string]$GoExe,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$CargoExe
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
New-Item -ItemType Directory -Path $output -Force | Out-Null
$vswhere=Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
$visualStudio=& $vswhere -latest -prerelease -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
if(-not $visualStudio){throw 'C++ tools are required for the process-exit fixture'}
$msvc=Get-ChildItem -LiteralPath (Join-Path $visualStudio 'VC/Tools/MSVC') -Directory | Sort-Object Name -Descending | Select-Object -First 1
$sdkRoot=Join-Path ${env:ProgramFiles(x86)} 'Windows Kits/10'
$sdk=Get-ChildItem -LiteralPath (Join-Path $sdkRoot 'Lib') -Directory | Sort-Object Name -Descending | Select-Object -First 1
$compiler=Join-Path $msvc.FullName 'bin/Hostx64/x64/cl.exe'
$fixture=Join-Path $output 'slow-detach.dll'
$arguments=@('/nologo','/LD','/W4','/WX','/GS-',('/Fo'+(Join-Path $output 'slow-detach.obj')),
    '/I',(Join-Path $msvc.FullName 'include'),'/I',(Join-Path $sdkRoot "Include/$($sdk.Name)/ucrt"),
    '/I',(Join-Path $sdkRoot "Include/$($sdk.Name)/shared"),'/I',(Join-Path $sdkRoot "Include/$($sdk.Name)/um"),
    (Join-Path $root 'tests/process-lifetime/slow-detach.c'),'/link','/NODEFAULTLIB','/ENTRY:DllMain',('/OUT:'+$fixture),
    (Join-Path $sdk.FullName 'um/x64/kernel32.lib'))
$priorPath=$env:PATH
$priorFixture=$env:WINONE_EXIT_FIXTURE_DLL
$priorTemp=$env:TEMP
$priorTmp=$env:TMP
try {
    $env:PATH=(Split-Path -Parent $compiler)+';'+$env:PATH
    & $compiler @arguments
    if($LASTEXITCODE -ne 0){throw 'Native detach fixture failed to build'}
    $env:WINONE_EXIT_FIXTURE_DLL=$fixture
    $env:TEMP=$output;$env:TMP=$output
    & $GoExe test ./internal/processlifetime ./modules/always-on-top/cmd/worker -count=1 -timeout 20s
    if($LASTEXITCODE -ne 0){throw 'Go process finalization failed'}
    if($CargoExe){
        & (Join-Path $PSScriptRoot 'build-desktop-rust.ps1') -GoExe $GoExe -CargoExe $CargoExe -OutputDirectory (Join-Path $output 'rust') -TestOnly -FrontendAlreadyBuilt
        if($LASTEXITCODE -ne 0){throw 'Rust process finalization failed'}
    }
    Write-Output 'Native DLL detach and saved-state finalization verified'
} finally {
    $env:PATH=$priorPath;$env:WINONE_EXIT_FIXTURE_DLL=$priorFixture;$env:TEMP=$priorTemp;$env:TMP=$priorTmp
}
