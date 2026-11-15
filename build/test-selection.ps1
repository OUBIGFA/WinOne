param([string]$GoExe='go', [string]$OutputDirectory='_temp/selection-checks', [string]$Run='Selection|Gesture|Keyboard|Clipboard|Reader|Watcher|CompatibilityCopy|CopyInput')
# Offline selection routing, input, clipboard ownership and reader cancellation.
# This entry does not opt into hooks, physical input, UI or network tests.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$priorTemp=$env:TEMP;$priorTmp=$env:TMP
New-Item -ItemType Directory -Path $output -Force | Out-Null
Push-Location $root
try {
    $env:TEMP=$output;$env:TMP=$output
    & $GoExe test -p 2 ./internal/selection ./apps/host -run $Run -count=1 -v
    if($LASTEXITCODE-ne 0){throw 'Selection regression failed'}
    & $GoExe vet ./internal/selection ./apps/host
    if($LASTEXITCODE-ne 0){throw 'Selection static checks failed'}
} finally {
    $env:TEMP=$priorTemp;$env:TMP=$priorTmp
    Pop-Location
}
