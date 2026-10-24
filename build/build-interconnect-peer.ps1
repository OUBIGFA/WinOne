param([string]$GoExe='go',[string]$OutputDirectory='_temp/interconnect/build',[switch]$Test,[string]$TestDirectory='_temp/interconnect-peer-build-tests')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $OutputDirectory
$go=(Get-Command $GoExe -ErrorAction Stop).Source
$output=[IO.Path]::GetFullPath($OutputDirectory)
& node (Join-Path $PSScriptRoot 'verify-import-notices.mjs')
if($LASTEXITCODE -ne 0){throw 'Peer source verification failed'}
& (Join-Path $PSScriptRoot 'verify-interconnect-transfer-notices.ps1') -GoExe $go -Runtime peer
$priorCache=$env:GOCACHE
$priorRoot=$env:GOROOT
try {
 $env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
 # An inherited GOROOT can point at another registered toolchain; the
 # selected binary must resolve its own compiler.
 $env:GOROOT=''
 New-Item -ItemType Directory -Path $output -Force | Out-Null
 Push-Location (Join-Path $root 'modules/interconnect/peer')
 try {
  if($Test){
   # The live-share test starts sibling LAN/transfer executables. Compile it
   # into an isolated bundle so the integration case cannot silently skip.
   $testBundle=if([IO.Path]::IsPathRooted($TestDirectory)){[IO.Path]::GetFullPath($TestDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $TestDirectory))}
   & (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $testBundle
   New-Item -ItemType Directory -Path $testBundle -Force | Out-Null
   foreach($name in @('WinOne.Interconnect.Lan.exe','WinOne.Interconnect.Transfer.exe')){Copy-Item -LiteralPath (Join-Path $output $name) -Destination (Join-Path $testBundle $name) -Force}
   $testExe=Join-Path $testBundle 'WinOne.Interconnect.Peer.Tests.exe'
   & $go test -mod=readonly -c -o $testExe .
   if($LASTEXITCODE -ne 0){throw 'Peer test build failed'}
   & $testExe '-test.v' '-test.timeout=120s'
   if($LASTEXITCODE -ne 0){throw 'Peer integration tests failed'}
  }
  & $go build -mod=readonly -trimpath -ldflags '-s -w' -o (Join-Path $output 'WinOne.Interconnect.Peer.exe') .
  if($LASTEXITCODE -ne 0){throw 'Peer build failed'}
 }finally{Pop-Location}
}finally{$env:GOCACHE=$priorCache;$env:GOROOT=$priorRoot}
