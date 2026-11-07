param([string]$SourceDirectory='dist/resources/sources/ecdict-39cfb96',[string]$OutputDirectory='dist/resources/assets/learning',[string]$PythonExe='py',[string]$Downloader='aria2-next')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$source=[IO.Path]::GetFullPath((Join-Path $root $SourceDirectory))
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $source -ReusableCache
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output -ReusableCache
$catalog=Get-Content -LiteralPath (Join-Path $root 'third_party/learning-data.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$license=Join-Path $root ('third_party/'+$catalog.licenseFile)
if((Get-FileHash -LiteralPath $license -Algorithm SHA256).Hash.ToLowerInvariant() -ne $catalog.licenseSHA256){throw 'ECDICT license mismatch'}
New-Item -ItemType Directory -Path $source,$output -Force | Out-Null
$csv=Join-Path $source 'ecdict.csv'
if(-not(Test-Path -LiteralPath $csv)){
 & $Downloader -d $source -o ecdict.csv --auto-file-renaming=false --allow-overwrite=false $catalog.source
 if($LASTEXITCODE -ne 0){throw 'ECDICT download failed'}
}
if((Get-FileHash -LiteralPath $csv -Algorithm SHA256).Hash.ToLowerInvariant() -ne $catalog.sourceSHA256){throw 'ECDICT source mismatch'}
$db=Join-Path $output 'ecdict.db'
$manifest=Join-Path $output 'manifest.json'
$builder=Join-Path $PSScriptRoot 'learning_dictionary.py'
$builderHash=(Get-FileHash -LiteralPath $builder -Algorithm SHA256).Hash.ToLowerInvariant()
if((Test-Path -LiteralPath $manifest) -and (Test-Path -LiteralPath $db)){
 $prior=Get-Content -LiteralPath $manifest -Raw -Encoding UTF8 | ConvertFrom-Json
 if($prior.sourceSHA256 -eq $catalog.sourceSHA256 -and $prior.builderSHA256 -eq $builderHash -and $prior.sha256 -eq (Get-FileHash -LiteralPath $db -Algorithm SHA256).Hash.ToLowerInvariant()) {Write-Output 'Learning dictionary cache verified';return}
 throw 'Existing dictionary cache differs. Use a new task output directory and verify it before replacing the cache.'
}
$oldUtf=$env:PYTHONUTF8
try{
 $env:PYTHONUTF8='1'
 $result=& $PythonExe -B $builder $csv $db --source-sha256 $catalog.sourceSHA256
 if($LASTEXITCODE -ne 0){throw 'Learning dictionary build failed'}
 $metadata=$result|ConvertFrom-Json
 $record=[ordered]@{format=1;sourceCommit=$catalog.commit;sourceSHA256=$catalog.sourceSHA256;builderSHA256=$builderHash;sha256=$metadata.sha256;entries=$metadata.entries;bytes=$metadata.bytes}
 [IO.File]::WriteAllText($manifest,($record|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
 Copy-Item -LiteralPath $license -Destination (Join-Path $output 'ECDICT-LICENSE.txt')
 Write-Output ($record|ConvertTo-Json -Compress)
}finally{$env:PYTHONUTF8=$oldUtf}
