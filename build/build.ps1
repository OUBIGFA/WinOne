param([string]$GoExe = 'go', [string]$CargoExe = 'cargo', [string]$Downloader, [switch]$SkipInstall, [string]$OutputDirectory = '_temp/build/current', [switch]$RunStartupProbes, [string]$UpdateRepository = '', [string]$ShareXSourceDirectory, [string]$RustTargetDirectory='dist/resources/cache/rust-desktop', [ValidateSet('Rust','Legacy','MyGo')][string]$DesktopImplementation='MyGo', [string]$OcrRuntimeDirectory=$env:WINONE_OCR_RUNTIME_DIRECTORY, [string]$MagpieSourceDirectory=$env:WINONE_MAGPIE_SOURCE_DIRECTORY, [string]$PluginSourceDirectory=$env:WINONE_AI_PLUGIN_SOURCE_DIRECTORY, [string]$BunExe=$env:WINONE_AI_BUN_EXE, [string]$ResultGoExe=$env:WINONE_RESULT_GO_EXE, [string]$DiagramTargetDirectory='dist/resources/cache/rust-diagrams', [string]$SelectionTargetDirectory='dist/resources/cache/rust-selection', [string]$ModuleTrustFile='', [switch]$CompileOnly, [string]$NodeExe = 'node')
$ErrorActionPreference = 'Stop'
# Explicit unattended build mode. Runtime/behavior tests are selected separately;
# source, license, compiler, static and payload verification still run here.
if($CompileOnly -and $RunStartupProbes){throw 'CompileOnly cannot run startup probes'}
$root = Split-Path -Parent $PSScriptRoot
if(-not $OcrRuntimeDirectory -or -not $MagpieSourceDirectory){throw 'Pass OcrRuntimeDirectory (build/prepare-ocr.ps1 output) and the pinned MagpieSourceDirectory'}
$buildVersion = (Get-Content -LiteralPath (Join-Path $root 'apps/desktop/frontend/package.json') -Raw -Encoding UTF8 | ConvertFrom-Json).version
# These values enter Go linker arguments, so reject URLs, whitespace and shell syntax.
if ($UpdateRepository -and ($UpdateRepository -notmatch '^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?/[A-Za-z0-9_.-]{1,100}$' -or ($UpdateRepository.Split('/')[1] -in @('.', '..')))) { throw 'UpdateRepository must be an owner/repository name' }
if ($buildVersion -notmatch '^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$') { throw 'Invalid build version' }
$moduleKeys=''
if($ModuleTrustFile){
    $trustBytes=[IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $ModuleTrustFile).Path)
    if($trustBytes.Length -gt 65536){throw 'Module trust configuration exceeds size limit'}
    $moduleKeys=[Convert]::ToBase64String($trustBytes)
}
Set-Location $root
$output = if ([IO.Path]::IsPathRooted($OutputDirectory)) { [IO.Path]::GetFullPath($OutputDirectory) } else { [IO.Path]::GetFullPath((Join-Path $root $OutputDirectory)) }
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
# Build only into a Git-ignored tree; keep alternate previews out of tracked source.
& git check-ignore -q --no-index -- (Join-Path $output 'build-output.probe')
if ($LASTEXITCODE -ne 0) { throw 'OutputDirectory must be inside a Git-ignored directory such as _temp/<task>/build' }
New-Item -ItemType Directory -Path $output -Force | Out-Null
$licenseDirectory = Join-Path $output 'licenses'
New-Item -ItemType Directory -Path $licenseDirectory -Force | Out-Null
Get-ChildItem -LiteralPath (Join-Path $root 'third_party/licenses') -File | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination $licenseDirectory -Force }
$env:npm_config_cache = Join-Path $root 'dist/resources/cache/npm'
function CheckExit { if ($LASTEXITCODE -ne 0) { throw "Command failed: $LASTEXITCODE" } }
# Vite may write a module-directive warning to stderr while still succeeding.
# Judge native tools by their exit code so warnings stay visible but do not
# abort the build; a real failure still throws.
function Invoke-NativeCommand([string]$Executable,[string[]]$Arguments,[string]$Failure){
    $previous=$ErrorActionPreference
    try{$ErrorActionPreference='Continue';& $Executable @Arguments 2>&1 | ForEach-Object {$_.ToString()};$code=$LASTEXITCODE}finally{$ErrorActionPreference=$previous}
    if($code -ne 0){throw "$Failure (exit $code)"}
}
if (-not $SkipInstall) { & npm.cmd ci --prefix apps/desktop/frontend --no-audit --no-fund; CheckExit }
# Intake checks run before compilers. Packaging repeats them against binaries;
# it must never be the first time a missing external license is discovered.
& $NodeExe (Join-Path $PSScriptRoot 'verify-frontend-notices.mjs'); CheckExit
& $NodeExe (Join-Path $PSScriptRoot 'verify-import-notices.mjs'); CheckExit
& (Join-Path $PSScriptRoot 'build-provider-icons.ps1') -CargoExe $CargoExe -GoExe $GoExe -TargetDirectory $DiagramTargetDirectory -WorkDirectory (Join-Path (Split-Path -Parent $output) 'provider-icons'); CheckExit
# The React frontend serves only the Rust and legacy desktops: MyGo ships none
# of it, so its build neither tests nor bundles it.
if($DesktopImplementation -ne 'MyGo'){
    # The UI suite runs within vitest.config.ts's worker budget, alone at this
    # point of the build.
    if(-not $CompileOnly){Invoke-NativeCommand 'npm.cmd' @('test','--prefix','apps/desktop/frontend','--','--pool=threads') 'Frontend tests failed'}
    Invoke-NativeCommand 'npm.cmd' @('run','build','--prefix','apps/desktop/frontend') 'Frontend build failed'

}
Push-Location third_party/systray
try { if(-not $CompileOnly){& $GoExe test ./...; CheckExit} } finally { Pop-Location }
& $GoExe run ./build/icon -out internal/branding/icon.ico -native-png internal/branding/logo-native.png; CheckExit
& (Join-Path $PSScriptRoot 'build-always-on-top.ps1') -OutputDirectory (Join-Path $output 'always-on-top')
& $GoExe run github.com/akavel/rsrc@v0.10.2 -manifest build/windows/app.manifest -ico internal/branding/icon.ico -o modules/always-on-top/cmd/worker/resource_windows.syso; CheckExit
& $GoExe build -trimpath -ldflags '-s -w -H windowsgui' -o (Join-Path $output 'always-on-top/WinOne.AlwaysOnTop.exe') ./modules/always-on-top/cmd/worker; CheckExit
$sharex = if($ShareXSourceDirectory){(Resolve-Path -LiteralPath $ShareXSourceDirectory).Path}else{& (Join-Path $PSScriptRoot 'prepare-sharex.ps1') -Downloader $Downloader}
if($ShareXSourceDirectory){
    # Refresh the host import when a prepared source cache has moved directories.
    . (Join-Path $PSScriptRoot 'sharex-native-host.ps1')
    Initialize-ShareXNativeHost -SourceDirectory $sharex
}
# Keep each build's test executable separate from other/failed test processes.
# A lingering fixture can otherwise lock the shared bin output on Windows.
$captureTests = Join-Path (Split-Path -Parent $output) 'capture-unit-tests'
if(-not $CompileOnly){& dotnet build tests/capture-unit/CaptureTests.csproj -c Release "-p:ShareXSourceDirectory=$sharex" -o $captureTests --nologo; CheckExit}
# The scrolling checks drive real windows; only build/test-native-context.ps1
# runs them, so they are compiled with the other system probes.
if ($RunStartupProbes) { & dotnet build tests/capture-scroll/ScrollTests.csproj -c Release "-p:ShareXSourceDirectory=$sharex" --nologo; CheckExit }
if(-not $CompileOnly){Invoke-NativeCommand 'dotnet' @((Join-Path $captureTests 'WinOne.Capture.Tests.dll'),(Join-Path (Split-Path -Parent $output) 'capture-test-data')) 'Capture unit tests failed'}
& dotnet publish modules/capture/WinOne.Capture.csproj -c Release -r win-x64 --self-contained true "-p:ShareXSourceDirectory=$sharex" -o (Join-Path $output 'capture') --nologo; CheckExit
& (Join-Path $PSScriptRoot 'build-recording.ps1') -CargoExe $CargoExe -OutputDirectory (Join-Path $output 'capture')
& (Join-Path $PSScriptRoot 'copy-ocr.ps1') -RuntimeDirectory $OcrRuntimeDirectory -CaptureDirectory (Join-Path $output 'capture')
& (Join-Path $OcrRuntimeDirectory 'python/python.exe') (Join-Path $PSScriptRoot 'verify-anki.py') --runtime (Join-Path $root 'dist/resources/runtimes/anki-26.09.4'); CheckExit
& (Join-Path $PSScriptRoot 'copy-anki.ps1') -BundleDirectory $output
& (Join-Path $PSScriptRoot 'update-ocr-inventory.ps1') -RuntimeDirectory (Join-Path $output 'capture/Ocr') -Check
# Current Magpie requires Go 1.26.8 or newer. Reuse the already-selected result
# toolchain and restore the Host compiler's environment after this build.
$aiGo=if($ResultGoExe){$ResultGoExe}else{$GoExe}
$aiPriorRoot=$env:GOROOT
try {
    $env:GOROOT=& $aiGo env GOROOT
    # An inherited GOROOT can point at the Host compiler; the selected binary
    # resolves its own standard library when that override is cleared.
    $env:GOROOT=''
    & (Join-Path $PSScriptRoot 'build-ai.ps1') -MagpieSourceDirectory $MagpieSourceDirectory -PluginSourceDirectory $PluginSourceDirectory -BunExe $BunExe -OutputDirectory (Join-Path $output 'shared/ai') -GoExe $aiGo -Test:(-not $CompileOnly)
} finally { $env:GOROOT=$aiPriorRoot }
$strokeBuild = Join-Path (Split-Path -Parent $output) 'stroke-build'
& (Join-Path $PSScriptRoot 'build-stroke.ps1') -OutputDirectory $strokeBuild -Test:(-not $CompileOnly)
New-Item -ItemType Directory -Path (Join-Path $output 'stroke') -Force | Out-Null
foreach ($file in @('WinOne.Stroke.exe','Stroke.Base.dll','Base.dll','Stroke.dll','Stroke-LICENSE.txt','schema.json','defaults.json','migrations.json')) { Copy-Item -LiteralPath (Join-Path $strokeBuild $file) -Destination (Join-Path $output 'stroke') -Force }
if(-not $ResultGoExe){$ResultGoExe=$GoExe}
$learningPrepare=@{PythonExe=(Join-Path $OcrRuntimeDirectory 'python/python.exe')}
if($Downloader){$learningPrepare.Downloader=$Downloader}
& (Join-Path $PSScriptRoot 'prepare-learning.ps1') @learningPrepare
& (Join-Path $PSScriptRoot 'build-results.ps1') -OutputDirectory (Join-Path $output 'shared/ui') -GoExe $ResultGoExe -SkipTests:$CompileOnly
& (Join-Path $PSScriptRoot 'build-diagrams.ps1') -OutputDirectory (Join-Path $output 'shared/ui') -CargoExe $CargoExe -GoExe $GoExe -TargetDirectory $DiagramTargetDirectory -SkipTests:$CompileOnly
& (Join-Path $PSScriptRoot 'build-selection.ps1') -OutputDirectory (Join-Path $output 'selection') -CargoExe $CargoExe -GoExe $GoExe -TargetDirectory $SelectionTargetDirectory -SkipTests:$CompileOnly
& (Join-Path $PSScriptRoot 'build-interconnect-lan.ps1') -OutputDirectory (Join-Path $output 'shared/interconnect') -CargoExe $CargoExe -Test:(-not $CompileOnly)
& (Join-Path $PSScriptRoot 'build-interconnect-transfer.ps1') -OutputDirectory (Join-Path $output 'shared/interconnect') -GoExe $ResultGoExe -Test:(-not $CompileOnly) -TestDirectory (Join-Path (Split-Path -Parent $output) 'interconnect-transfer-tests')
& (Join-Path $PSScriptRoot 'build-interconnect-peer.ps1') -OutputDirectory (Join-Path $output 'shared/interconnect') -GoExe $ResultGoExe -Test:(-not $CompileOnly) -TestDirectory (Join-Path (Split-Path -Parent $output) 'interconnect-peer-tests')
& (Join-Path $PSScriptRoot 'build-capture-regions.ps1') -OutputDirectory (Join-Path $output 'capture')
if(-not $CompileOnly){
$regionLifetimeProbe = Join-Path (Split-Path -Parent $output) 'capture-regions-lifetime-probe'
& (Join-Path $PSScriptRoot 'build-capture-regions.ps1') -TestLifetime -OutputDirectory $regionLifetimeProbe
Invoke-NativeCommand 'dotnet' @((Join-Path $captureTests 'WinOne.Capture.Tests.dll'),'--region-helper-lifetime',(Join-Path $regionLifetimeProbe 'WinOne.Capture.Regions.LifetimeProbe.exe')) 'Capture region lifetime test failed'
}
if ($RunStartupProbes) {
    $previousRegionHelper = $env:WINONE_REGION_HELPER
    try {
        $env:WINONE_REGION_HELPER = Join-Path $output 'capture/WinOne.Capture.Regions.exe'
        & dotnet (Join-Path $captureTests 'WinOne.Capture.Tests.dll') --native-window-regions; CheckExit
    } finally { $env:WINONE_REGION_HELPER = $previousRegionHelper }
}
& $GoExe run github.com/akavel/rsrc@v0.10.2 -manifest build/windows/app.manifest -ico internal/branding/icon.ico -o apps/desktop/resource_windows.syso; CheckExit
& $GoExe run github.com/akavel/rsrc@v0.10.2 -manifest build/windows/app.manifest -ico internal/branding/icon.ico -o apps/host/resource_windows.syso; CheckExit
# Enumerate only maintained Go module sources.
$goModulePackages=@('./modules/always-on-top/...','./modules/stroke/...')
$goSourcePackages=@('./apps/...','./build/...','./internal/...','./tests/...')+$goModulePackages
$goTestPackages=@(& $GoExe list @goSourcePackages); CheckExit
if(-not $CompileOnly){& $GoExe test @goTestPackages; CheckExit}
& $GoExe vet @goSourcePackages; CheckExit
& (Join-Path $PSScriptRoot 'build-wintray.ps1') -GoExe $GoExe -OutputDirectory $output -RunConsoleChecks:$RunStartupProbes -SkipTests:$CompileOnly
CheckExit
# Host is a background application even when Task Scheduler launches it directly.
& (Join-Path $PSScriptRoot 'build-host.ps1') -GoExe $GoExe -OutputDirectory $output -Version $buildVersion -UpdateRepository $UpdateRepository -ModulePublishers $moduleKeys; CheckExit
if($DesktopImplementation -eq 'Rust'){
    & (Join-Path $PSScriptRoot 'build-desktop-rust.ps1') -CargoExe $CargoExe -GoExe $GoExe -OutputDirectory $output -TargetDirectory $RustTargetDirectory -ComponentUI -FrontendAlreadyBuilt -UpdateRepository $UpdateRepository
    CheckExit
}elseif($DesktopImplementation -eq 'MyGo'){
    & (Join-Path $PSScriptRoot 'build-desktop-mygo.ps1') -GoExe $ResultGoExe -OutputDirectory $output -Version $buildVersion -UpdateRepository $UpdateRepository -SkipTests:$CompileOnly -NodeExe $NodeExe
    CheckExit
}else{
    & $GoExe build -trimpath -tags desktop,production -ldflags "-H windowsgui -X main.updateRepository=$UpdateRepository -X main.updateVersion=$buildVersion" -o (Join-Path $output 'WinOne.exe') ./apps/desktop; CheckExit
}
# Keep these gates reusable after an interrupted native verification; do not
# rebuild already-verified compilers/modules merely to rerun the final checks.
& (Join-Path $PSScriptRoot 'verify-built-bundle.ps1') -BundleDirectory $output -Version $buildVersion -GoExe $GoExe -ResultGoExe $ResultGoExe -CargoExe $CargoExe -DesktopImplementation $DesktopImplementation -RunStartupProbes:$RunStartupProbes -SkipRuntimeChecks:$CompileOnly
