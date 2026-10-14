param([string]$CargoExe='cargo',[switch]$Update,[string]$BundleDirectory,[ValidateSet('diagram','selection')][string]$Runtime='diagram')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$cargo=(Get-Command $CargoExe -ErrorAction Stop).Source
$priorRustc=$env:RUSTC
function Hash([string]$path){(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()}
try{
 $env:RUSTC=Join-Path (Split-Path $cargo -Parent) 'rustc.exe'
 $manifest=if($Runtime -eq 'selection'){'modules/selection/native/Cargo.toml'}else{'apps/diagram-renderer/Cargo.toml'}
 $json=& $cargo metadata --locked --format-version 1 --filter-platform x86_64-pc-windows-msvc --manifest-path (Join-Path $root $manifest)
 if($LASTEXITCODE -ne 0){throw 'Cannot resolve diagram dependencies'}
 $metadata=$json|ConvertFrom-Json;$packages=@()
 foreach($package in $metadata.packages|Where-Object source|Sort-Object name,version){
  $directory=Split-Path $package.manifest_path -Parent
  $licenses=@(Get-ChildItem -LiteralPath $directory -File|Where-Object Name -Match '^(LICENSE|LICENCE|COPYING|NOTICE)([._-].*)?$'|Sort-Object Name)
  if(-not $licenses.Count){throw "Missing diagram dependency notice: $($package.name)"}
  $notices=@()
  foreach($license in $licenses){
   $name=($Runtime+'-'+$package.name+'-'+$package.version+'-'+$license.Name+'.txt')-replace '[^A-Za-z0-9._-]','-'
   $file='licenses/'+$name;$destination=Join-Path $root ('third_party/'+$file);$hash=Hash $license.FullName
   if($Update){Copy-Item -LiteralPath $license.FullName -Destination $destination -Force}
   if((Hash $destination) -ne $hash){throw "Diagram license changed: $name"}
   if($BundleDirectory -and (Hash (Join-Path $BundleDirectory $file)) -ne $hash){throw "Diagram packaged license mismatch: $name"}
   $notices+=[ordered]@{file=$file;sha256=$hash}
  }
  $packages+=[ordered]@{name=$package.name;version=$package.version;source=$package.source;license=$package.license;notices=$notices}
 }
 $catalog=[ordered]@{schemaVersion=1;packages=$packages}
 $path=Join-Path $root ('third_party/'+$Runtime+'-notices.json');$encoded=$catalog|ConvertTo-Json -Depth 8
 if($Update){[IO.File]::WriteAllText($path,$encoded,[Text.UTF8Encoding]::new($false))}
 if((Get-Content -Raw -Encoding UTF8 $path).Trim() -ne $encoded.Trim()){throw 'Diagram dependency catalog changed'}
 if($BundleDirectory){Copy-Item -LiteralPath $path -Destination (Join-Path $BundleDirectory ('licenses/'+$Runtime+'-notices.txt')) -Force}
 Write-Output ($Runtime+" dependency notices verified: "+$packages.Count)
}finally{$env:RUSTC=$priorRustc}
