param(
 [Parameter(Mandatory)][string]$OutputDirectory,
 [string]$Downloader='aria2-next'
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'runtime.probe')
if($LASTEXITCODE -ne 0){throw 'OCR preparation output must be Git-ignored'}
$downloads=Join-Path $output 'downloads';$runtime=Join-Path $output 'Ocr';$models=Join-Path $runtime 'models'
New-Item -ItemType Directory -Force -Path $downloads,$runtime,$models | Out-Null
$catalog=Get-Content -LiteralPath (Join-Path $root 'third_party/ocr-runtime.json') -Raw -Encoding UTF8 | ConvertFrom-Json
function Get-VerifiedAsset([string]$Name,[string]$Url,[string]$Hash){
 $path=Join-Path $downloads $Name
 if(-not(Test-Path -LiteralPath $path)){
  & $Downloader -d $downloads -o $Name --check-certificate=true --auto-file-renaming=false --allow-overwrite=false $Url | Out-Host
  if($LASTEXITCODE -ne 0){throw "OCR asset download failed: $Name"}
 }
 if((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $Hash){throw "OCR asset checksum mismatch: $Name"}
 return $path
}
$archive=Get-VerifiedAsset $catalog.sdk.file $catalog.sdk.url $catalog.sdk.sha256
$sdk=Join-Path $output 'sdk'
Expand-Archive -LiteralPath $archive -DestinationPath $sdk -Force
Copy-Item -LiteralPath (Join-Path $sdk $catalog.sdk.library) -Destination (Join-Path $runtime 'rapidocrlite.dll') -Force
foreach($file in $catalog.files){
 $target=Join-Path $runtime $file.path
 if($file.url){
  $download=Get-VerifiedAsset ([IO.Path]::GetFileName($file.path)) $file.url $file.sha256
  Copy-Item -LiteralPath $download -Destination $target -Force
 }elseif($file.sourceFile){
  Copy-Item -LiteralPath (Join-Path $root $file.sourceFile) -Destination $target -Force
 }
 if((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant() -ne $file.sha256){throw "Prepared OCR checksum mismatch: $($file.path)"}
}
$runtime
