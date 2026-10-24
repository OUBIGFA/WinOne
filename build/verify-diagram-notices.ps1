param([string]$CargoExe='cargo',[switch]$Update,[string]$BundleDirectory,[ValidateSet('diagram','selection','recording','search','interconnect-lan')][string]$Runtime='diagram',[string]$ManifestPath)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$cargo=(Get-Command $CargoExe -ErrorAction Stop).Source
$priorRustc=$env:RUSTC
function Hash([string]$path){(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()}
try{
 $env:RUSTC=Join-Path (Split-Path $cargo -Parent) 'rustc.exe'
 $manifest=switch($Runtime){'interconnect-lan'{'third_party/file-share/src-tauri/Cargo.toml'} 'selection'{'modules/selection/native/Cargo.toml'} 'recording'{'modules/recording/Cargo.toml'} 'search'{'dist/resources/sources/ripgrep-15.2.0/Cargo.toml'} default{'apps/diagram-renderer/Cargo.toml'}}
 $selected=if($ManifestPath){[IO.Path]::GetFullPath($ManifestPath)}else{Join-Path $root $manifest}
 $json=& $cargo metadata --locked --format-version 1 --filter-platform x86_64-pc-windows-msvc --manifest-path $selected
 if($LASTEXITCODE -ne 0){throw 'Cannot resolve diagram dependencies'}
 $metadata=$json|ConvertFrom-Json;$packages=@()
 $searchPackages=@{}
 if($Runtime -eq 'search'){
  # Workspace metadata also lists optional PCRE2 and test members. Follow the
  # actual root dependency graph; WinOne builds the fixed-string/Rust-regex
  # binary without PCRE2, not those unrelated workspace targets.
  $nodes=@{};foreach($node in $metadata.resolve.nodes){$nodes[$node.id]=$node}
  $queue=New-Object 'System.Collections.Generic.Queue[string]';$queue.Enqueue($metadata.resolve.root)
  while($queue.Count){$id=$queue.Dequeue();if($searchPackages.ContainsKey($id)){continue};$searchPackages[$id]=$true;foreach($dep in $nodes[$id].deps){if(@($dep.dep_kinds|Where-Object {$_.kind -ne 'dev'}).Count){$queue.Enqueue($dep.pkg)}}}
 }
 foreach($package in $metadata.packages|Where-Object { if($Runtime -eq 'search'){$searchPackages.ContainsKey($_.id)}else{$_.source -or ($Runtime -eq 'recording' -and $_.name -eq 'gifski')} }|Sort-Object name,version){
  $directory=Split-Path $package.manifest_path -Parent
  $licenses=@(Get-ChildItem -LiteralPath $directory -File|Where-Object { $_.Name -Match '^(LICENSE|LICENCE|COPYING|COPYRIGHT|NOTICE)([._-].*)?$' -or ($Runtime -eq 'search' -and $_.Name -eq 'UNLICENSE') }|Sort-Object Name)
  $notices=@()
  if(-not $licenses.Count){
   # Some crate tarballs intentionally exclude their workspace LICENSE. An
   # audited exception must match the package's recorded git revision; never
   # synthesize a license from the SPDX name or silently accept missing text.
   $originsPath=Join-Path $root 'third_party/license-origins.json'
   if(-not(Test-Path -LiteralPath $originsPath)){throw "Missing dependency notice: $($package.name) $($package.version)"}
   $origins=Get-Content -LiteralPath $originsPath -Raw -Encoding UTF8|ConvertFrom-Json
   $origin=@($origins.packages|Where-Object {$_.name -eq $package.name -and $_.version -eq $package.version})
   if($origin.Count -ne 1){throw "Missing dependency notice origin: $($package.name) $($package.version)"}
   $revision=(Get-Content -LiteralPath (Join-Path $directory '.cargo_vcs_info.json') -Raw -Encoding UTF8|ConvertFrom-Json).git.sha1
   $notice=$origin[0]
   if($notice.commit -ne $revision -or $notice.repository -ne $package.repository -or $notice.file -notmatch '^licenses/[A-Za-z0-9._-]+\.txt$' -or (Hash (Join-Path $root ('third_party/'+$notice.file))) -ne $notice.sha256){throw "Unverified upstream license: $($package.name)"}
   $notices+=[ordered]@{file=$notice.file;sha256=$notice.sha256;sourceURL=$notice.sourceURL}
   if($notice.PSObject.Properties.Name -contains 'declaration'){
    $d=$notice.declaration
    if($d.sourceFile -ne 'Cargo.toml.orig' -or $d.file -notmatch '^licenses/[A-Za-z0-9._-]+\.txt$' -or $d.license -ne $package.license -or (Hash (Join-Path $directory $d.sourceFile)) -ne $d.sha256 -or (Hash (Join-Path $root ('third_party/'+$d.file))) -ne $d.sha256){throw "Upstream license declaration changed: $($package.name)"}
    $notices+=[ordered]@{file=$d.file;sha256=$d.sha256}
   }
  }
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
 if($BundleDirectory){
  foreach($package in $packages){foreach($notice in $package.notices){if((Hash (Join-Path $BundleDirectory $notice.file)) -ne $notice.sha256){throw "Packaged dependency license mismatch: $($notice.file)"}}}
  Copy-Item -LiteralPath $path -Destination (Join-Path $BundleDirectory ('licenses/'+$Runtime+'-notices.txt')) -Force
 }
 Write-Output ($Runtime+" dependency notices verified: "+$packages.Count)
}finally{$env:RUSTC=$priorRustc}
