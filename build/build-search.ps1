param(
 [Parameter(Mandatory)][string]$OutputDirectory,
 [string]$CargoExe='cargo',
 [string]$SourceDirectory='dist/resources/sources/ripgrep-15.2.0',
 [string]$TargetDirectory='dist/resources/cache/rust-search'
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
function Absolute([string]$p){if([IO.Path]::IsPathRooted($p)){[IO.Path]::GetFullPath($p)}else{[IO.Path]::GetFullPath((Join-Path $root $p))}}
$output=Absolute $OutputDirectory;$source=Absolute $SourceDirectory;$target=Absolute $TargetDirectory
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $target -ReusableCache
$record=Get-Content -LiteralPath (Join-Path $root 'third_party/ripgrep.json') -Raw -Encoding UTF8|ConvertFrom-Json
if(-not(Test-Path -LiteralPath (Join-Path $source 'Cargo.toml'))){throw 'Prepare the pinned ripgrep source cache before building Chat'}
$head=& git -C $source rev-parse HEAD
if($LASTEXITCODE -ne 0 -or $head -ne $record.commit){throw 'Search source revision differs from third_party/ripgrep.json'}
if((Get-FileHash -LiteralPath (Join-Path $source 'Cargo.lock')).Hash -ne (Get-FileHash -LiteralPath (Join-Path $root $record.lock)).Hash){throw 'Search dependency lock changed'}
$cargo=(Get-Command $CargoExe -ErrorAction Stop).Source
$priorPath=$env:PATH;$priorLib=$env:LIB;$priorRustc=$env:RUSTC
try {
 $env:RUSTC=Join-Path (Split-Path $cargo -Parent) 'rustc.exe'
 $vswhere=Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
 if(Test-Path -LiteralPath $vswhere){
  $installation=& $vswhere -latest -prerelease -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
  if($installation){
   $msvc=Get-ChildItem (Join-Path $installation 'VC/Tools/MSVC') -Directory|Sort-Object Name -Descending|Select-Object -First 1
   $sdk=Get-ChildItem (Join-Path ${env:ProgramFiles(x86)} 'Windows Kits/10/Lib') -Directory|Sort-Object Name -Descending|Select-Object -First 1
   $env:PATH=(Join-Path $msvc.FullName 'bin/Hostx64/x64')+';'+$priorPath
   $env:LIB=@((Join-Path $msvc.FullName 'lib/x64'),(Join-Path $sdk.FullName 'um/x64'),(Join-Path $sdk.FullName 'ucrt/x64'),$priorLib)-join ';'
  }
 }
 & (Join-Path $PSScriptRoot 'verify-diagram-notices.ps1') -Runtime search -CargoExe $cargo -ManifestPath (Join-Path $source 'Cargo.toml')
 $nativePreference=$ErrorActionPreference
 try {
  $ErrorActionPreference='Continue'
  & $cargo build --release --locked --no-default-features --bin rg --manifest-path (Join-Path $source 'Cargo.toml') --target-dir $target 2>&1 | ForEach-Object {$_.ToString()}
  $buildExit=$LASTEXITCODE
 } finally { $ErrorActionPreference=$nativePreference }
 if($buildExit -ne 0){throw 'Bundled search build failed'}
 $executable=Join-Path $target 'release/rg.exe'
 $version=@(& $executable --version)
 if($LASTEXITCODE -ne 0 -or $version[0] -notlike ('ripgrep '+$record.version+'*') -or -not($version -contains 'features:-pcre2')){throw 'Unexpected search version or optional PCRE2 feature'}
 New-Item -ItemType Directory -Path $output -Force|Out-Null
 Copy-Item -LiteralPath $executable -Destination (Join-Path $output 'rg.exe') -Force
 # A standalone Chat build carries the same license tree as the full bundle.
 $notices=Get-Content -LiteralPath (Join-Path $root $record.dependencyNotices) -Raw -Encoding UTF8|ConvertFrom-Json
 New-Item -ItemType Directory -Path (Join-Path $output 'licenses') -Force|Out-Null
 foreach($item in $notices.packages){foreach($notice in $item.notices){Copy-Item -LiteralPath (Join-Path $root ('third_party/'+$notice.file)) -Destination (Join-Path $output $notice.file) -Force}}
 Copy-Item -LiteralPath (Join-Path $root $record.dependencyNotices) -Destination (Join-Path $output 'licenses/search-notices.json') -Force
 [IO.File]::WriteAllText((Join-Path $output 'search-runtime.json'),(@{version=$record.version;commit=$record.commit;sha256=(Get-FileHash -LiteralPath $executable -Algorithm SHA256).Hash.ToLowerInvariant();features=@()}|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
}finally{$env:PATH=$priorPath;$env:LIB=$priorLib;$env:RUSTC=$priorRustc}
