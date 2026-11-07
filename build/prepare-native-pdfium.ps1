param(
 [Parameter(Mandatory)][string]$OutputDirectory,
 [string]$LibraryPath,
 [string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe',
 [string]$CacheDirectory='dist/resources/runtimes/native-pdfium'
)
# Prepare the digest-verified, unchanged PDFium DLL and the small native PDF
# helper, using this repository's compiler/cache. No global installation.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$record=Get-Content -LiteralPath (Join-Path $root 'third_party/native-pdfium.json') -Raw -Encoding UTF8|ConvertFrom-Json
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){$OutputDirectory}else{Join-Path $root $OutputDirectory}
$cache=if([IO.Path]::IsPathRooted($CacheDirectory)){$CacheDirectory}else{Join-Path $root $CacheDirectory}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $cache -ReusableCache
& node (Join-Path $PSScriptRoot 'verify-native-pdfium.mjs')
if($LASTEXITCODE-ne 0){throw 'PDFium source notices failed'}
$destination=Join-Path $cache ($record.library.sha256+'/'+$record.library.name)
if(-not(Test-Path -LiteralPath $destination)){
 if(-not $LibraryPath){
  $download=Join-Path $root ('_temp/native-pdfium-download/'+[guid]::NewGuid().ToString('N'))
  & (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $download
  New-Item -ItemType Directory -Force $download|Out-Null
  $archive=Join-Path $download 'pdfium.tgz';Invoke-WebRequest -UseBasicParsing $record.archive.url -OutFile $archive
  if((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()-ne $record.archive.sha256){throw 'PDFium release archive checksum mismatch'}
  & tar -xf $archive -C $download bin/pdfium.dll
  if($LASTEXITCODE-ne 0){throw 'PDFium library could not be extracted'}
  $LibraryPath=Join-Path $download 'bin/pdfium.dll'
 }
 & node (Join-Path $PSScriptRoot 'verify-native-pdfium.mjs') --library $LibraryPath
 if($LASTEXITCODE-ne 0){throw 'PDFium library checksum mismatch'}
 New-Item -ItemType Directory -Force (Split-Path -Parent $destination)|Out-Null
 Copy-Item -LiteralPath $LibraryPath -Destination $destination
}
& node (Join-Path $PSScriptRoot 'verify-native-pdfium.mjs') --library $destination
if($LASTEXITCODE-ne 0){throw 'Existing PDFium cache differs; preserving it for investigation'}
New-Item -ItemType Directory -Force $output|Out-Null
Copy-Item -LiteralPath $destination -Destination (Join-Path $output $record.library.name)
$licenses=Join-Path $output 'licenses';New-Item -ItemType Directory -Force $licenses|Out-Null
foreach($notice in $record.notices){Copy-Item -LiteralPath (Join-Path $root ('third_party/'+$notice.file)) -Destination $licenses -Force}
Copy-Item -LiteralPath (Join-Path $root 'third_party/native-pdfium.json') -Destination (Join-Path $licenses 'native-pdfium.txt') -Force
$go=if([IO.Path]::IsPathRooted($GoExe)){$GoExe}else{Join-Path $root $GoExe}
$prior=$env:GOROOT
try{
 $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go)
 & $go -C (Join-Path $root 'apps/desktop-mygo') build -trimpath -ldflags '-s -w' -o (Join-Path $output 'WinOne.PDF.exe') ./cmd/pdfworker
 if($LASTEXITCODE-ne 0){throw 'Native PDF helper build failed'}
}finally{$env:GOROOT=$prior}
& node (Join-Path $PSScriptRoot 'verify-native-pdfium.mjs') --bundle $output
if($LASTEXITCODE-ne 0){throw 'Native PDFium bundle verification failed'}
Write-Output (Join-Path $output 'WinOne.PDF.exe')
