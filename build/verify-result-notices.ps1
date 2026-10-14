param([Parameter(Mandatory)][string]$BundleDirectory,[string]$GoExe='go')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$source=Join-Path $root 'third_party/result-ui-notices.json'
$catalog=Get-Content -LiteralPath $source -Encoding UTF8 -Raw | ConvertFrom-Json
$records=@{}
foreach($component in $catalog.components){
 $key=$component.kind+'|'+$component.name+'|'+$component.version
 if($records.ContainsKey($key) -or @($component.notices).Count -eq 0){throw "Invalid result notice record: $key"}
 $records[$key]=$component
 foreach($notice in $component.notices){
  foreach($base in @((Join-Path $root 'third_party'),$BundleDirectory)){
   if((Get-FileHash -LiteralPath (Join-Path $base $notice.file) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $notice.sha256){throw "Result notice mismatch: $($notice.file)"}
  }
 }
}
# Inspect the actual executable, including its independent Go toolchain.
$info=@(& $GoExe version -m (Join-Path $BundleDirectory 'shared/ui/WinOne.Results.exe'))
if($LASTEXITCODE -ne 0 -or $info.Count -eq 0){throw 'Cannot inspect result UI dependencies'}
$version=($info[0] -split '\s+')[-1]
if(-not $records.ContainsKey('toolchain|Go|'+$version)){throw 'Result UI toolchain notice mismatch'}
foreach($line in $info){
 $parts=$line.Trim() -split '\s+'
 if($parts[0] -eq 'dep' -and $parts[1] -ne 'winone' -and -not $records.ContainsKey('go|'+$parts[1]+'|'+$parts[2])){throw "Missing result dependency notice: $line"}
}
Copy-Item -LiteralPath $source -Destination (Join-Path $BundleDirectory 'licenses/result-ui-notices.txt') -Force
$iconsPath=Join-Path $root 'third_party/provider-icons.json'
$icons=Get-Content -LiteralPath $iconsPath -Encoding UTF8 -Raw | ConvertFrom-Json
foreach($icon in $icons.files){
 if($icon.file -notmatch '^[a-z0-9-]+\.(svg|png)$'){throw 'Invalid provider icon inventory path'}
 $asset=Join-Path $root ('apps/result-ui/assets/providers/'+$icon.file)
 if((Get-FileHash -LiteralPath $asset -Algorithm SHA256).Hash.ToLowerInvariant() -ne $icon.sha256){throw "Provider icon changed: $($icon.file)"}
}
if(@(Get-ChildItem -LiteralPath (Join-Path $root 'apps/result-ui/assets/providers') -File).Count -ne @($icons.files).Count){throw 'Unrecorded provider icon'}
$sampledRoot=Join-Path $root 'apps/result-ui/assets/provider-bitmaps'
$sampled=Get-Content -LiteralPath (Join-Path $sampledRoot 'manifest.json') -Encoding UTF8 -Raw|ConvertFrom-Json
if($sampled.renderer -ne 'resvg 0.47.0' -or $sampled.size -ne 128){throw 'Unexpected icon rasterizer'}
foreach($entry in $sampled.files){
 $original=@($icons.files|Where-Object {$_.file -eq ($entry.name+'.svg')})
 if($original.Count -ne 1 -or $original[0].sha256 -ne $entry.sourceSHA256 -or (Get-FileHash -LiteralPath (Join-Path $sampledRoot ($entry.name+'.png')) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $entry.pngSHA256){throw 'Derived icon does not match its pinned source'}
}
if(@($sampled.files).Count -ne @($icons.files|Where-Object {$_.file.EndsWith('.svg')}).Count -or @(Get-ChildItem -LiteralPath $sampledRoot -Filter '*.png').Count -ne @($sampled.files).Count){throw 'Incomplete derived icon inventory'}
Copy-Item -LiteralPath (Join-Path $sampledRoot 'manifest.json') -Destination (Join-Path $BundleDirectory 'licenses/provider-bitmaps.txt') -Force
Copy-Item -LiteralPath $iconsPath -Destination (Join-Path $BundleDirectory 'licenses/provider-icons.txt') -Force
Write-Output "Result UI dependency notices verified: $($catalog.components.Count) records"
