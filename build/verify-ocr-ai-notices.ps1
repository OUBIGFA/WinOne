param([Parameter(Mandatory)][string]$BundleDirectory,[string]$GoExe='go')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$catalog=Get-Content -LiteralPath (Join-Path $root 'third_party/ocr-ai-notices.json') -Encoding UTF8 -Raw | ConvertFrom-Json
$records=@{}
foreach($component in $catalog.components){
 $key=$component.kind+'|'+$component.name+'|'+$component.version
 if($records.ContainsKey($key) -or @($component.notices).Count -eq 0){throw "Invalid OCR/AI notice record: $key"}
 $records[$key]=$component
 foreach($notice in $component.notices){
  foreach($base in @((Join-Path $root 'third_party'),$BundleDirectory)){
   $path=Join-Path $base $notice.file
   if((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $notice.sha256){throw "OCR/AI notice mismatch: $($notice.file)"}
  }
 }
}
$info=& $GoExe version -m (Join-Path $BundleDirectory 'shared/ai/WinOne.AI.exe')
if($LASTEXITCODE -ne 0){throw 'Cannot inspect magpie executable dependencies'}
foreach($line in $info){
 $parts=$line.Trim() -split '\s+'
 if($parts[0] -eq 'dep'){
  if(-not $records.ContainsKey('magpie-go|'+$parts[1]+'|'+$parts[2])){throw "Missing magpie dependency notice: $line"}
 }
}
$ocr=Get-Content -LiteralPath (Join-Path $root 'third_party/ocr-runtime.json') -Raw -Encoding UTF8 | ConvertFrom-Json
if(-not $records.ContainsKey('native|RapidOCRLite|'+$ocr.version)){throw 'Missing RapidOCRLite notice'}
foreach($file in $ocr.files){
 $path=Join-Path $BundleDirectory ('capture/Ocr/'+$file.path)
 if((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $file.sha256){throw "Packaged OCR runtime mismatch: $($file.path)"}
}
$plugins=Get-Content -LiteralPath (Join-Path $root 'third_party/ai-plugins.json') -Raw -Encoding UTF8|ConvertFrom-Json
foreach($plugin in $plugins.plugins){
 foreach($file in $plugin.files){
  $relative='shared/ai/plugins/'+$plugin.directory.Substring(9)+'/'+$file.path.Substring($plugin.directory.Length+1)
  $path=Join-Path $BundleDirectory $relative
  if(-not(Test-Path -LiteralPath $path)){throw "Account plugin resource missing: $relative"}
 }
}
if(-not(Test-Path -LiteralPath (Join-Path $BundleDirectory 'shared/ai/runtime/bun.exe'))){throw 'Shared Bun runtime missing'}
foreach($notice in @($plugins.license,$plugins.bun.license)){
 foreach($base in @((Join-Path $root 'third_party'),$BundleDirectory)){
  if(-not(Test-Path -LiteralPath (Join-Path $base $notice.file))){throw "Account runtime license missing: $($notice.file)"}
 }
}
Copy-Item -LiteralPath (Join-Path $root 'third_party/ai-plugins.json') -Destination (Join-Path $BundleDirectory 'licenses/ai-plugins.txt') -Force
Copy-Item -LiteralPath (Join-Path $root 'third_party/ocr-ai-notices.json') -Destination (Join-Path $BundleDirectory 'licenses/ocr-ai-notices.txt') -Force
Write-Output "OCR and AI dependency notices verified: $($catalog.components.Count) records"
