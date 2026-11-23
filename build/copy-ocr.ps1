param([Parameter(Mandatory)][string]$RuntimeDirectory,[Parameter(Mandatory)][string]$CaptureDirectory)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$runtime=(Resolve-Path -LiteralPath $RuntimeDirectory).Path
$destination=Join-Path ([IO.Path]::GetFullPath($CaptureDirectory)) 'Ocr'
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $destination
$catalog=Get-Content -LiteralPath (Join-Path $root 'third_party/ocr-runtime.json') -Raw -Encoding UTF8 | ConvertFrom-Json
# Copy the reviewed four-file runtime only. Reused build outputs must not retain
# a previous Python payload; choose a fresh output when its inventory differs.
if(Test-Path -LiteralPath $destination){
 foreach($file in Get-ChildItem -LiteralPath $destination -Recurse -File){
  $relative=$file.FullName.Substring($destination.Length+1).Replace('\','/')
  if($relative -notin $catalog.files.path){throw "Stale OCR payload; use a new capture output: $relative"}
 }
}
foreach($file in $catalog.files){
 $source=Join-Path $runtime $file.path
 if((Get-FileHash -LiteralPath $source -Algorithm SHA256).Hash.ToLowerInvariant() -ne $file.sha256){throw "OCR runtime checksum mismatch: $($file.path)"}
 $target=Join-Path $destination $file.path
 New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target) | Out-Null
 Copy-Item -LiteralPath $source -Destination $target -Force
}
