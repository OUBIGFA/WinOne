param([string]$GoExe='go', [string]$OutputDirectory='_temp/transport-checks')
# Local framing, named-pipe deadlines and caller cancellation need no network
# listeners or desktop interaction. Keep this focused regression reproducible.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$priorTemp=$env:TEMP;$priorTmp=$env:TMP
New-Item -ItemType Directory -Path $output -Force | Out-Null
Push-Location $root
try {
    $env:TEMP=$output;$env:TMP=$output
    & $GoExe test ./internal/transport ./internal/protocol -count=1 -v
    if($LASTEXITCODE-ne 0){throw 'Local transport regression failed'}
    & $GoExe vet ./internal/transport ./internal/protocol
    if($LASTEXITCODE-ne 0){throw 'Local transport static checks failed'}
} finally {
    $env:TEMP=$priorTemp;$env:TMP=$priorTmp
    Pop-Location
}
