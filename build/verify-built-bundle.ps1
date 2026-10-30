# The unified build and interrupted-build recovery share the same final gates.
# Use on a fully compiled bundle only; this does not replace compiler/unit tests.
param(
 [Parameter(Mandatory)][string]$BundleDirectory,
 [Parameter(Mandatory)][string]$Version,
 [string]$GoExe='go',
 [string]$ResultGoExe='go',
 [string]$CargoExe='cargo',
 [ValidateSet('Rust','Legacy')][string]$DesktopImplementation='Rust',
 [switch]$RunStartupProbes
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath($BundleDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
function CheckExit { if($LASTEXITCODE -ne 0){throw "Bundle verification failed: $LASTEXITCODE"} }
Push-Location $root
$keys=@('WINONE_HOST_EXE','WINONE_CAPTURE_EXE','WINONE_PROBE_BUNDLE','WINONE_DESKTOP_PROBE_EXE','WINONE_NATIVE_HOTKEY_TEST','TEMP','TMP')
$previous=@{};foreach($key in $keys){$previous[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
try{
 # Keep the native fixtures outside the payload, without sharing the machine's
 # temporary directory with unrelated builds or user application processes.
 $tests=Join-Path (Split-Path $output -Parent) ('bundle-verification-'+[guid]::NewGuid().ToString('N'))
 New-Item -ItemType Directory -Path $tests -Force | Out-Null
 $env:TEMP=$tests;$env:TMP=$tests
 $env:WINONE_HOST_EXE=Join-Path $output 'WinOne.Host.exe'
 $env:WINONE_CAPTURE_EXE=Join-Path $output 'capture/WinOne.Capture.exe'
 if($RunStartupProbes){$env:WINONE_PROBE_BUNDLE=$output;$env:WINONE_DESKTOP_PROBE_EXE=Join-Path $output 'WinOne.exe';$env:WINONE_NATIVE_HOTKEY_TEST='1'}
 & $GoExe test ./internal/startupprobe -count=1; CheckExit
 & $GoExe test ./apps/host -run '^TestNativeConfigurationProbe$|^TestNativeRecordingReleasesAndRestoresRegistration$' -count=1; CheckExit
 & $GoExe test ./tests/integration -count=1; CheckExit
 $currentVersion=(Get-Content -LiteralPath (Join-Path $root 'apps/desktop/frontend/package.json') -Raw -Encoding UTF8|ConvertFrom-Json).version
 if($currentVersion -ne $Version){throw 'Version changed during build; rebuild before packaging'}
 & node (Join-Path $root 'tests/notices/frontend.test.mjs'); CheckExit
 & (Join-Path $PSScriptRoot 'verify-notices.ps1') -GoExe $GoExe -BundleDirectory $output
 & (Join-Path $PSScriptRoot 'verify-ocr-ai-notices.ps1') -GoExe $GoExe -BundleDirectory $output
 & (Join-Path $PSScriptRoot 'verify-result-notices.ps1') -GoExe $ResultGoExe -BundleDirectory $output
 & (Join-Path $PSScriptRoot 'verify-interconnect-transfer-notices.ps1') -GoExe $ResultGoExe -BundleDirectory $output
 & (Join-Path $PSScriptRoot 'verify-interconnect-transfer-notices.ps1') -GoExe $ResultGoExe -BundleDirectory $output -Runtime peer
 foreach($runtime in @('diagram','selection','recording','search')){& (Join-Path $PSScriptRoot 'verify-diagram-notices.ps1') -CargoExe $CargoExe -BundleDirectory $output -Runtime $runtime}
 if($DesktopImplementation -eq 'Rust'){& (Join-Path $PSScriptRoot 'verify-rust-notices.ps1') -CargoExe $CargoExe -BundleDirectory $output}
 & $GoExe run ./build/package --record-build --bundle $output --license (Join-Path $root 'LICENSE') --version $Version; CheckExit
 Write-Output "Build verified: $output/WinOne.exe"
}finally{
 foreach($key in $keys){[Environment]::SetEnvironmentVariable($key,$previous[$key],'Process')}
 Pop-Location
}
