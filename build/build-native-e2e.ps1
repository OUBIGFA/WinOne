param([Parameter(Mandatory)][string]$OutputDirectory,[string]$BundleDirectory='_temp/migration-regressions-20261104/bundle',[string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe')
# Test-owned full application; native automation is linked ONLY into this
# separate executable. The real Host and pages use an isolated profile/pipe.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
New-Item -ItemType Directory -Path $output -Force|Out-Null
$go=(Get-Command (Join-Path $root $GoExe)).Source
$prior=$env:GOROOT;$cache=$env:GOCACHE
try {
 $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go);$env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
 & (Join-Path $PSScriptRoot 'prepare-mygo-filedrop.ps1') -GoExe $go
 # Reuse verified runtime assets. Never prepare settings in a real profile.
 Get-ChildItem -LiteralPath $BundleDirectory | Copy-Item -Destination $output -Recurse -Force
 & $go build -o (Join-Path $output 'WinOne.NativeAutomation.exe') ./tests/native-automation
 if($LASTEXITCODE -ne 0){throw 'Native automation client build failed'}
 Push-Location (Join-Path $root 'apps/desktop-mygo')
 try { & $go build -tags 'nativee2e' -o (Join-Path $output 'WinOne.exe') ./cmd/winone; if($LASTEXITCODE -ne 0){throw 'Native E2E app build failed'} }finally{Pop-Location}
}finally{$env:GOROOT=$prior;$env:GOCACHE=$cache}
