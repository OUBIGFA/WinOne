param(
    [Parameter(Mandatory)][string]$ShareXSourceDirectory,
    [string]$DotnetExe = 'dotnet'
)
# Offscreen bitmap assertions only: no capture, clipboard or desktop changes.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$source = (Resolve-Path -LiteralPath $ShareXSourceDirectory).Path
. (Join-Path $PSScriptRoot 'sharex-native-host.ps1')
Initialize-ShareXNativeHost $source
& $DotnetExe build (Join-Path $root 'tests/capture-unit/CaptureTests.csproj') -c Release "-p:ShareXSourceDirectory=$source" --nologo -v:q
if ($LASTEXITCODE -ne 0) { throw '截图菜单回归构建失败，请检查编译输出。' }
& $DotnetExe (Join-Path $root 'tests/capture-unit/bin/Release/net10.0-windows/WinOne.Capture.Tests.dll') --menu-arrows
if ($LASTEXITCODE -ne 0) { throw '截图菜单线性箭头回归失败，请检查像素断言。' }
