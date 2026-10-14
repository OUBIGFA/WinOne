param([Parameter(Mandatory)][string]$RuntimeDirectory,[switch]$Check)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$runtime=(Resolve-Path -LiteralPath $RuntimeDirectory).Path.TrimEnd('\')
$target=Join-Path $root 'internal/distribution/ocr_runtime_paths.txt'
[string[]]$files=@(Get-ChildItem -LiteralPath $runtime -Recurse -File | ForEach-Object {'capture/Ocr/'+$_.FullName.Substring($runtime.Length+1).Replace('\','/')})
[Array]::Sort($files,[StringComparer]::Ordinal)
foreach($file in $files){if($file -match '(__pycache__|\.pyc$|\.log$|\.tmp$)'){throw "Temporary file in OCR payload: $file"}}
$content=($files -join "`n")+"`n"
if($Check){if([IO.File]::ReadAllText($target).Replace("`r`n","`n") -ne $content){throw 'OCR runtime inventory changed; review dependencies and regenerate its pinned file list'}}
else{[IO.File]::WriteAllText($target,$content,[Text.UTF8Encoding]::new($false))}
