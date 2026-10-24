param([string]$CargoExe='cargo',[string]$OutputDirectory='_temp/interconnect/build',[string]$TargetDirectory='dist/resources/cache/rust-interconnect-lan',[switch]$Test)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $OutputDirectory
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $TargetDirectory -ReusableCache
$cargo=(Get-Command $CargoExe -ErrorAction Stop).Source
$priorRustc=$env:RUSTC
$priorCargo=$env:CARGO_HOME
function Invoke-InterconnectCargo([string[]]$Arguments){
 $previous=$ErrorActionPreference
 try{$ErrorActionPreference='Continue'; & $cargo @Arguments 2>&1 | ForEach-Object {$_.ToString()}; $code=$LASTEXITCODE}
 finally{$ErrorActionPreference=$previous}
 if($code -ne 0){throw "FileShare build or tests failed (exit $code)"}
}
try {
 $env:RUSTC=Join-Path (Split-Path $cargo -Parent) 'rustc.exe'
 $env:CARGO_HOME=Join-Path $root 'dist/resources/cache/cargo'
 & node (Join-Path $PSScriptRoot 'verify-import-notices.mjs')
 if($LASTEXITCODE -ne 0){throw 'Interconnect source notice verification failed'}
 & (Join-Path $PSScriptRoot 'verify-diagram-notices.ps1') -Runtime interconnect-lan -CargoExe $cargo
 $manifest=Join-Path $root 'third_party/file-share/src-tauri/Cargo.toml'
 if($Test){
  Invoke-InterconnectCargo @('test','--locked','--manifest-path',$manifest,'--target-dir',$TargetDirectory)
 }
 Invoke-InterconnectCargo @('build','--locked','--release','--manifest-path',$manifest,'--target-dir',$TargetDirectory,'--bin','winone-interconnect-lan')
 New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
 Copy-Item -LiteralPath (Join-Path $TargetDirectory 'release/winone-interconnect-lan.exe') -Destination (Join-Path $OutputDirectory 'WinOne.Interconnect.Lan.exe') -Force
} finally { $env:RUSTC=$priorRustc; $env:CARGO_HOME=$priorCargo }
