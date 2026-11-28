param([string]$GoExe='go',[string]$OutputDirectory='_temp/interconnect/build',[switch]$Test,[string]$TestDirectory='_temp/interconnect-worker-tests')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $OutputDirectory
$go=(Get-Command $GoExe -ErrorAction Stop).Source
$output=[IO.Path]::GetFullPath($OutputDirectory)
& node (Join-Path $PSScriptRoot 'verify-import-notices.mjs')
if($LASTEXITCODE -ne 0){throw 'Interconnect source verification failed'}
& (Join-Path $PSScriptRoot 'verify-interconnect-worker-notices.ps1') -GoExe $go
$priorCache=$env:GOCACHE
$priorRoot=$env:GOROOT
try {
 $env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
 $env:GOROOT=''
 New-Item -ItemType Directory -Path $output -Force | Out-Null
 Push-Location (Join-Path $root 'modules/interconnect')
 try {
  & $go build -mod=readonly -trimpath -ldflags '-s -w' -o (Join-Path $output 'WinOne.Interconnect.Worker.exe') ./cmd/worker
  if($LASTEXITCODE -ne 0){throw 'Interconnect worker build failed'}
  if($Test){
   # The unified build passes an absolute task directory. Join only relative
   # inputs; PowerShell otherwise prefixes the repository a second time.
   $testBundle=if([IO.Path]::IsPathRooted($TestDirectory)){[IO.Path]::GetFullPath($TestDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $TestDirectory))}
   & (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $testBundle
   New-Item -ItemType Directory -Path $testBundle -Force | Out-Null
   foreach($name in @('WinOne.Interconnect.Lan.exe','WinOne.Interconnect.Worker.exe')){Copy-Item -LiteralPath (Join-Path $output $name) -Destination $testBundle -Force}
   $priorTestRoot=$env:WINONE_TRANSFER_TEST_ROOT
   try{
    $env:WINONE_TRANSFER_TEST_ROOT=$testBundle
    foreach($package in @('transfer','peer','cmd/worker')){
     $name=$package.Replace('/','-')+'.test.exe'
     $testExe=Join-Path $testBundle $name
     & $go test -mod=readonly -c -o $testExe ('./'+$package)
     if($LASTEXITCODE -ne 0){throw "Interconnect test build failed: $package"}
     & $testExe '-test.v' '-test.timeout=120s'
     if($LASTEXITCODE -ne 0){throw "Interconnect tests failed: $package"}
    }
   }finally{$env:WINONE_TRANSFER_TEST_ROOT=$priorTestRoot}
  }
 }finally{Pop-Location}
}finally{$env:GOCACHE=$priorCache;$env:GOROOT=$priorRoot}
