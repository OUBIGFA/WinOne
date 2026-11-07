param(
 [Parameter(Mandatory)][string]$MyGoSourceDirectory,
 [string]$LibraryPath,
 [string]$OutputDirectory,
 [string]$CacheDirectory='dist/resources/runtimes/native-terminal'
)
# Prepare only the original library pinned by the MyGo package. Nothing is
# installed globally; tests/runtime must name this path explicitly so MyGo's
# development fallback never downloads into the user's own cache.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$spec=Get-Content -LiteralPath (Join-Path $root 'third_party/native-terminal.json') -Raw -Encoding UTF8|ConvertFrom-Json
$cache=if([IO.Path]::IsPathRooted($CacheDirectory)){$CacheDirectory}else{Join-Path $root $CacheDirectory}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $cache -ReusableCache
& node (Join-Path $PSScriptRoot 'verify-native-terminal.mjs') --mygo-source $MyGoSourceDirectory
if($LASTEXITCODE-ne 0){throw 'Native terminal source notices failed'}
$cacheHome=Join-Path $cache $spec.library.sha256
$destination=Join-Path $cacheHome $spec.library.name
if(Test-Path -LiteralPath $destination){
 if((Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash.ToLowerInvariant()-ne $spec.library.sha256){throw 'Existing native terminal cache differs; preserving it for investigation'}
}else{
 if(-not $LibraryPath){
  $download=Join-Path $root ('_temp/native-terminal-download/'+[guid]::NewGuid().ToString('N'))
  & (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $download
  New-Item -ItemType Directory -Force $download|Out-Null
  $LibraryPath=Join-Path $download $spec.library.name
  Invoke-WebRequest -UseBasicParsing $spec.library.url -OutFile $LibraryPath
 }
 if((Get-FileHash -LiteralPath $LibraryPath -Algorithm SHA256).Hash.ToLowerInvariant()-ne $spec.library.sha256){throw 'Native terminal release checksum mismatch'}
 New-Item -ItemType Directory -Force $cacheHome|Out-Null
 Copy-Item -LiteralPath $LibraryPath -Destination $destination
}
& node (Join-Path $PSScriptRoot 'verify-native-terminal.mjs') --mygo-source $MyGoSourceDirectory --library $destination
if($LASTEXITCODE-ne 0){throw 'Native terminal artifact verification failed'}
if($OutputDirectory){
 $output=if([IO.Path]::IsPathRooted($OutputDirectory)){$OutputDirectory}else{Join-Path $root $OutputDirectory}
 & (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
 New-Item -ItemType Directory -Force $output|Out-Null
 Copy-Item -LiteralPath $destination -Destination (Join-Path $output $spec.library.name)
 $licenses=Join-Path $output 'licenses';New-Item -ItemType Directory -Force $licenses|Out-Null
 foreach($component in $spec.components){foreach($notice in $component.notices){Copy-Item -LiteralPath (Join-Path $root ('third_party/'+$notice.file)) -Destination $licenses -Force}}
 Copy-Item -LiteralPath (Join-Path $root 'third_party/native-terminal.json') -Destination (Join-Path $licenses 'native-terminal.txt') -Force
 & node (Join-Path $PSScriptRoot 'verify-native-terminal.mjs') --bundle $output
 if($LASTEXITCODE-ne 0){throw 'Native terminal bundled verification failed'}
}
Write-Output $destination
