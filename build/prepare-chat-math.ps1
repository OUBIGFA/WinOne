# The full unmodified source package accompanies the generated font data. Reuse
# the pinned archive; only a missing cache is downloaded, never npm-installed.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$catalog=Get-Content -LiteralPath (Join-Path $root 'third_party/chat-math-notices.json') -Raw -Encoding UTF8|ConvertFrom-Json
foreach($resource in $catalog.resources){
 $target=Join-Path $root $resource.source
 & (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $target -ReusableCache
 if(-not (Test-Path -LiteralPath $target)){
  New-Item -ItemType Directory -Path (Split-Path -Parent $target) -Force|Out-Null
  $partial=$target+'.partial'
  Invoke-WebRequest -UseBasicParsing -Uri $resource.sourceURL -OutFile $partial
  if((Get-FileHash -LiteralPath $partial -Algorithm SHA256).Hash.ToLowerInvariant()-ne $resource.sha256){throw 'Math font source download hash mismatch'}
  Move-Item -LiteralPath $partial -Destination $target
 }
}
& node (Join-Path $PSScriptRoot 'verify-chat-math-notices.mjs')
if($LASTEXITCODE-ne 0){throw 'Math font source/license verification failed'}
