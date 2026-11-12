param([string]$GoExe='go', [string]$OutputDirectory='_temp/source-checks')
# This gate needs only tracked sources, Go and Node. Full runtime, native UI,
# OCR and installer acceptance remain in build.ps1 and verify-built-bundle.ps1.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$go=(Get-Command $GoExe -ErrorAction Stop).Source
function Invoke-SourceTool([string]$Executable,[string[]]$Arguments) {
    $tool=(Get-Command $Executable -ErrorAction Stop).Source
    $previous=$ErrorActionPreference
    try {
        $ErrorActionPreference='Continue'
        & $tool @Arguments 2>&1 | ForEach-Object { $_.ToString() }
        $code=$LASTEXITCODE
    } finally { $ErrorActionPreference=$previous }
    if($code -ne 0){throw "Source check failed ($code): $Executable $($Arguments -join ' ')"}
}
$priorTemp=$env:TEMP;$priorTmp=$env:TMP
Push-Location $root
try {
    & (Join-Path $PSScriptRoot 'verify-repository.ps1')
    New-Item -ItemType Directory -Path $output -Force | Out-Null
    $env:TEMP=$output;$env:TMP=$output
    Invoke-SourceTool $go @('run','./build/icon','-out','internal/branding/icon.ico','-native-png','internal/branding/logo-native.png')
    Invoke-SourceTool 'node' @('build/verify-import-notices.mjs')
    # The legacy Wails app embeds a separately built web frontend. Its tests,
    # and nested MyGo/native modules, run in their complete build entry points.
    $modulePackages=@('./modules/always-on-top/...','./modules/stroke/...')
    $packages=@('./apps/host','./build/...','./internal/...','./tests/...')+$modulePackages
    # Several packages launch real short-lived fixtures. Bound package
    # concurrency so those startup checks do not compete with dozens of peers.
    Invoke-SourceTool $go (@('test','-p','2','-count=1')+$packages)
    Invoke-SourceTool $go (@('vet')+$packages)
    foreach($module in @('third_party/systray','modules/wintray')) {
        Push-Location (Join-Path $root $module)
        try {
            Invoke-SourceTool $go @('test','-p','2','./...')
            # Match the complete build: systray uses Windows-owned GDI/message
            # addresses that the generic unsafeptr analyzer cannot classify.
            # Its native behavior tests run above; WinTray also has a vet gate.
            if($module -eq 'modules/wintray'){Invoke-SourceTool $go @('vet','./...')}
        } finally { Pop-Location }
    }
    & (Join-Path $PSScriptRoot 'build-host.ps1') -GoExe $go -OutputDirectory (Join-Path $output 'host')
    Write-Output 'Source tests, vet and Host build passed; full bundle acceptance is separate.'
} finally {
    $env:TEMP=$priorTemp;$env:TMP=$priorTmp
    Pop-Location
}
