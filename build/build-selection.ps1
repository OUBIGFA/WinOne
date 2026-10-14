param([Parameter(Mandatory)][string]$OutputDirectory,[string]$CargoExe='cargo',[string]$GoExe='go',[string]$TargetDirectory='dist/resources/cache/rust-selection')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
function Absolute([string]$path){if([IO.Path]::IsPathRooted($path)){[IO.Path]::GetFullPath($path)}else{[IO.Path]::GetFullPath((Join-Path $root $path))}}
$output=Absolute $OutputDirectory;$target=Absolute $TargetDirectory
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $target -ReusableCache
foreach($path in @($output,$target)){& git -C $root check-ignore -q --no-index -- (Join-Path $path 'build-output.probe');if($LASTEXITCODE -ne 0){throw 'Selection outputs must be ignored'}}
$cargo=(Get-Command $CargoExe -ErrorAction Stop).Source;$go=(Get-Command $GoExe -ErrorAction Stop).Source
$savedPath=$env:PATH;$savedLib=$env:LIB;$savedRustc=$env:RUSTC;$savedResource=$env:WINONE_SELECTION_RESOURCE
try{
 $env:RUSTC=Join-Path (Split-Path $cargo -Parent) 'rustc.exe'
 $vswhere=Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
 if(Test-Path -LiteralPath $vswhere){
  $installation=& $vswhere -latest -prerelease -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
  if($installation){
   $msvc=Get-ChildItem (Join-Path $installation 'VC/Tools/MSVC') -Directory|Sort-Object Name -Descending|Select-Object -First 1
   $sdk=Get-ChildItem (Join-Path ${env:ProgramFiles(x86)} 'Windows Kits/10/Lib') -Directory|Sort-Object Name -Descending|Select-Object -First 1
   if(-not $msvc -or -not $sdk){throw 'Windows C++ tools and SDK are required'}
   $env:PATH=(Join-Path $msvc.FullName 'bin/Hostx64/x64')+';'+$savedPath
   $env:LIB=@((Join-Path $msvc.FullName 'lib/x64'),(Join-Path $sdk.FullName 'um/x64'),(Join-Path $sdk.FullName 'ucrt/x64'),$savedLib)-join ';'
  }
 }
 New-Item -ItemType Directory -Path $output,$target -Force|Out-Null
 $resource=Join-Path $target 'selection-resource.syso'
 $ErrorActionPreference='Continue'
 & $go run github.com/akavel/rsrc@v0.10.2 -manifest (Join-Path $root 'build/windows/app.manifest') -ico (Join-Path $root 'internal/branding/icon.ico') -o $resource
 if($LASTEXITCODE -ne 0){throw 'Selection Windows resources failed'}
 $env:WINONE_SELECTION_RESOURCE=$resource
 & $cargo test --locked --manifest-path (Join-Path $root 'modules/selection/native/Cargo.toml') --target-dir $target
 if($LASTEXITCODE -ne 0){throw 'Selection renderer tests failed'}
 & $cargo build --release --locked --manifest-path (Join-Path $root 'modules/selection/native/Cargo.toml') --target-dir $target
 if($LASTEXITCODE -ne 0){throw 'Selection renderer build failed'}
 $ErrorActionPreference='Stop'
 Copy-Item -LiteralPath (Join-Path $target 'release/winone-selection-reader.exe') -Destination (Join-Path $output 'WinOne.Selection.exe') -Force
}finally{$env:PATH=$savedPath;$env:LIB=$savedLib;$env:RUSTC=$savedRustc;$env:WINONE_SELECTION_RESOURCE=$savedResource}
