param([Parameter(Mandatory)][string]$OutputDirectory,[string]$GoExe='go',[switch]$SkipTests)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
if(-not(Test-Path -LiteralPath (Join-Path $root 'apps/result-ui/assets/provider-bitmaps/manifest.json'))){throw 'Run build/build-provider-icons.ps1 before building the shared result UI'}
$go=(Get-Command $GoExe -ErrorAction Stop).Source
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'WinOne.Results.exe')
if($LASTEXITCODE -ne 0){throw 'Result UI output must be ignored'}
& node (Join-Path $PSScriptRoot 'generate-result-icons.mjs')
if($LASTEXITCODE -ne 0){throw 'Result icon generation failed'}
$oldRoot=$env:GOROOT;$oldCgo=$env:CGO_ENABLED;$oldFlags=$env:GOFLAGS
try {
 $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go);$env:CGO_ENABLED='0'
 $overlay=& (Join-Path $PSScriptRoot 'result-ui-overlay.ps1') -GoExe $go
 $env:GOFLAGS=($oldFlags+' "-modfile='+$overlay+'"').Trim()
 Push-Location (Join-Path $root 'apps/result-ui')
 try {
  if(-not $SkipTests){
   & $go test github.com/egoist/mygo/ui -run '^Test(WinOneFastSelection|SelectableText|TextArea)' -count=1
   if($LASTEXITCODE -ne 0){throw 'MyGo fast selection regression failed'}
   & $go test ./... -count=1;if($LASTEXITCODE -ne 0){throw 'Result UI tests failed'}
  }
  & $go vet ./...;if($LASTEXITCODE -ne 0){throw 'Result UI vet failed'}
  & $go run github.com/akavel/rsrc@v0.10.2 -manifest (Join-Path $root 'build/windows/app.manifest') -ico (Join-Path $root 'internal/branding/icon.ico') -o cmd/winone-results/resource_windows.syso
  if($LASTEXITCODE -ne 0){throw 'Result UI resource generation failed'}
  New-Item -ItemType Directory -Path $output -Force|Out-Null
  & $go build -trimpath -ldflags '-s -w -H windowsgui' -o (Join-Path $output 'WinOne.Results.exe') ./cmd/winone-results
  if($LASTEXITCODE -ne 0){throw 'Result UI build failed'}
 }finally{Pop-Location}
}finally{$env:GOROOT=$oldRoot;$env:CGO_ENABLED=$oldCgo;$env:GOFLAGS=$oldFlags}
