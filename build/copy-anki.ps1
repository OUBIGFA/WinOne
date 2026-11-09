param(
    [string]$RuntimeDirectory='dist/resources/runtimes/anki-26.09.4',
    [Parameter(Mandatory)][string]$BundleDirectory
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $BundleDirectory
$runtime=(Resolve-Path -LiteralPath $RuntimeDirectory).Path
$manifest=Get-Content -LiteralPath (Join-Path $runtime 'manifest.json') -Raw -Encoding UTF8|ConvertFrom-Json
$source=Join-Path $root 'third_party/anki/source.json'
if((Get-FileHash -LiteralPath $source).Hash.ToLowerInvariant() -ne $manifest.sourceSHA256){throw 'Anki runtime was built from another source lock'}
$destination=Join-Path $BundleDirectory 'shared/anki'
New-Item -ItemType Directory -Force $destination | Out-Null
foreach($file in $manifest.files.PSObject.Properties){
    $original=Join-Path $runtime $file.Name
    if((Get-FileHash -LiteralPath $original).Hash.ToLowerInvariant() -ne $file.Value){throw ('Anki runtime changed: '+$file.Name)}
    $target=Join-Path $destination $file.Name
    New-Item -ItemType Directory -Force (Split-Path -Parent $target)|Out-Null
    Copy-Item -LiteralPath $original -Destination $target -Force
}
Get-ChildItem -LiteralPath (Join-Path $root 'modules/anki') -File -Filter '*.py' |
    Where-Object Name -NotLike 'test_*' | Copy-Item -Destination $destination -Force
$connect=Join-Path $destination 'connect'
New-Item -ItemType Directory -Force $connect|Out-Null
Get-ChildItem -LiteralPath (Join-Path $root 'third_party/anki-connect/plugin') -File | Copy-Item -Destination $connect -Force
$notices=Join-Path $destination 'licenses'
New-Item -ItemType Directory -Force $notices | Out-Null
foreach($name in @('anki','anki-connect')){
    $sourceDirectory=Join-Path $root ('third_party/'+$name)
    foreach($file in Get-ChildItem -LiteralPath $sourceDirectory -File -Recurse){
        $relative=$file.FullName.Substring($sourceDirectory.Length+1)
        if($relative -match '(^|[\\/])__pycache__([\\/]|$)' -or $file.Extension -in @('.pyc','.pyo')){continue}
        $target=Join-Path $notices ($name+'/'+$relative)
        New-Item -ItemType Directory -Force (Split-Path -Parent $target)|Out-Null
        Copy-Item -LiteralPath $file.FullName -Destination $target -Force
    }
}
Copy-Item -LiteralPath (Join-Path $runtime 'manifest.json') -Destination $destination -Force
# Shared Python is already included by copy-ocr; Anki adds its own site directory.
if(-not (Test-Path -LiteralPath (Join-Path $BundleDirectory 'capture/Ocr/python/python.exe'))){throw 'Shared Python runtime is missing from the bundle'}
