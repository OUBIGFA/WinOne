param([Parameter(Mandatory)][string]$OutputDirectory,[string]$ResourceDirectory='dist/resources/assets/learning',[switch]$VerifyOnly)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$source=if([IO.Path]::IsPathRooted($ResourceDirectory)){$ResourceDirectory}else{Join-Path $root $ResourceDirectory}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $OutputDirectory
$catalog=Get-Content -LiteralPath (Join-Path $root 'third_party/learning-data.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$manifest=Get-Content -LiteralPath (Join-Path $source 'manifest.json') -Raw -Encoding UTF8 | ConvertFrom-Json
if($manifest.format -ne 1 -or $manifest.sourceCommit -ne $catalog.commit -or $manifest.sourceSHA256 -ne $catalog.sourceSHA256){throw 'Learning data source differs from the pinned catalog'}
if($manifest.builderSHA256 -ne (Get-FileHash -LiteralPath (Join-Path $PSScriptRoot 'learning_dictionary.py') -Algorithm SHA256).Hash.ToLowerInvariant()){throw 'Learning dictionary builder changed; rebuild the resource cache'}
if($manifest.sha256 -ne (Get-FileHash -LiteralPath (Join-Path $source 'ecdict.db') -Algorithm SHA256).Hash.ToLowerInvariant()){throw 'Learning dictionary checksum mismatch'}
if($catalog.licenseSHA256 -ne (Get-FileHash -LiteralPath (Join-Path $source 'ECDICT-LICENSE.txt') -Algorithm SHA256).Hash.ToLowerInvariant()){throw 'Learning dictionary license mismatch'}
if(-not $VerifyOnly){
 New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
 foreach($name in @('ecdict.db','manifest.json','ECDICT-LICENSE.txt')){Copy-Item -LiteralPath (Join-Path $source $name) -Destination (Join-Path $OutputDirectory $name) -Force}
}
Write-Output "Learning dictionary verified: $($manifest.entries) entries, $($manifest.bytes) bytes"
