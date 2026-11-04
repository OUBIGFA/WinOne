param(
    [ValidateSet('desktop-mygo','result-ui')][string[]]$Modules=@('desktop-mygo','result-ui'),
    [string]$Packages='./...',
    [string]$Run='.',
    [string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe'
)
# Targeted native regression entry; production builds still run the full suite.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$goPath=if([IO.Path]::IsPathRooted($GoExe)){$GoExe}else{Join-Path $root $GoExe}
$go=(Get-Command $goPath).Source
$priorRoot=$env:GOROOT; $priorCache=$env:GOCACHE
try {
    $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go)
    $env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
    & (Join-Path $PSScriptRoot 'prepare-mygo-filedrop.ps1') -GoExe $go
    foreach($module in $Modules) {
        Push-Location (Join-Path $root ('apps/'+$module))
        try {
            & $go test $Packages -run $Run -count=1 -timeout 180s
            if($LASTEXITCODE -ne 0){throw "$module tests failed"}
        } finally { Pop-Location }
    }
} finally { $env:GOROOT=$priorRoot; $env:GOCACHE=$priorCache }
