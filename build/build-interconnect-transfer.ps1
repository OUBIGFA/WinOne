param([string]$GoExe='go',[string]$OutputDirectory='_temp/interconnect/build',[switch]$Test,[string]$TestDirectory='_temp/interconnect-transfer-build-tests')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $OutputDirectory
$go=(Get-Command $GoExe -ErrorAction Stop).Source
$output=[IO.Path]::GetFullPath($OutputDirectory)
& node (Join-Path $PSScriptRoot 'verify-import-notices.mjs')
if($LASTEXITCODE -ne 0){throw 'Transfer source verification failed'}
& (Join-Path $PSScriptRoot 'verify-interconnect-transfer-notices.ps1') -GoExe $go
$priorCache=$env:GOCACHE
$priorRoot=$env:GOROOT
try {
 $env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
 # An inherited GOROOT can point at another registered toolchain; the
 # selected binary must resolve its own compiler.
 $env:GOROOT=''
 New-Item -ItemType Directory -Path $output -Force | Out-Null
 if($Test){
  # Integration tests write received files; keep them outside the bundle.
  $testRoot=if([IO.Path]::IsPathRooted($TestDirectory)){$TestDirectory}else{Join-Path $root $TestDirectory}
  $env:WINONE_TRANSFER_TEST_ROOT=[IO.Path]::GetFullPath($testRoot)
  & (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $env:WINONE_TRANSFER_TEST_ROOT
  New-Item -ItemType Directory -Path $env:WINONE_TRANSFER_TEST_ROOT -Force | Out-Null
 }
 Push-Location (Join-Path $root 'modules/interconnect/transfer')
 try {
  if($Test){& $go test -mod=readonly -timeout 120s .;if($LASTEXITCODE -ne 0){throw 'Transfer integration tests failed'}}
  & $go build -mod=readonly -trimpath -ldflags '-s -w' -o (Join-Path $output 'WinOne.Interconnect.Transfer.exe') .
  if($LASTEXITCODE -ne 0){throw 'Transfer build failed'}
 }finally{Pop-Location}
}finally{$env:GOCACHE=$priorCache;$env:GOROOT=$priorRoot}
