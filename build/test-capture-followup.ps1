param([ValidateSet('ocr','results','all')][string]$Suite='all', [string]$Filter='.', [string]$GoExe='go', [string]$ResultGoExe,
      [string]$OutputDirectory='_temp/capture-followup-tests')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$prior=@{};foreach($name in @('GOROOT','GOCACHE','GOMODCACHE','GOPATH','GOTOOLCHAIN','CGO_ENABLED','TEMP','TMP')){$prior[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
$go=(Get-Command $GoExe -ErrorAction Stop).Source
if(-not $ResultGoExe){$ResultGoExe=$go}
$env:GOROOT=Split-Path -Parent (Split-Path -Parent $go)
$env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
$env:GOMODCACHE=Join-Path $root 'dist/resources/cache/go-mod'
$env:GOPATH=Join-Path $root 'dist/resources/cache/go-path'
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){$OutputDirectory}else{Join-Path $root $OutputDirectory}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
New-Item -ItemType Directory -Force -Path $output | Out-Null
try {
 $env:TEMP=$output;$env:TMP=$output
 if($Suite -in @('ocr','all')) {
  Push-Location $root
  try { & $go test ./internal/ai ./apps/host -run $Filter -count=1; if($LASTEXITCODE){throw 'OCR/Host tests failed'} } finally {Pop-Location}
 }
 if($Suite -in @('results','all')) {
  $env:GOROOT=Split-Path -Parent (Split-Path -Parent $ResultGoExe)
  $env:GOTOOLCHAIN='local';$env:CGO_ENABLED='0'
  Push-Location (Join-Path $root 'apps/result-ui')
  try { & $ResultGoExe test ./... -run $Filter -count=1; if($LASTEXITCODE){throw 'Result tests failed'} } finally {Pop-Location}
 }
} finally { foreach($name in $prior.Keys){[Environment]::SetEnvironmentVariable($name,$prior[$name],'Process')} }
