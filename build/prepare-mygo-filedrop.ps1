param([Parameter(Mandatory)][string]$GoExe)
# Copy the verified, pinned module into a repository-owned source cache, then
# apply ONLY the declared native hooks. Never edit the user's Go module cache.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$source=Join-Path $root 'third_party/mygo-winone'
$specPath=Join-Path $source 'source.json'
$spec=Get-Content -LiteralPath $specPath -Raw -Encoding utf8|ConvertFrom-Json
$module=(& $GoExe mod download -json ($spec.module+'@'+$spec.version))|ConvertFrom-Json
if($LASTEXITCODE -ne 0 -or $module.Sum -ne $spec.moduleSum){throw 'MyGo source version/checksum mismatch'}
if((Get-FileHash -LiteralPath (Join-Path $root ('third_party/'+$spec.notice))).Hash.ToLowerInvariant() -ne $spec.licenseSHA256){throw 'MyGo license mismatch'}
$output=Join-Path $root ('dist/resources/sources/mygo-winone-'+$spec.version)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output -ReusableCache
$inputs=@((Get-FileHash -LiteralPath $specPath).Hash)
foreach($addition in $spec.additions){$inputs+=(Get-FileHash -LiteralPath (Join-Path $source $addition.source)).Hash}
$stamp=Join-Path $output 'winone-source-cache.json'
if(Test-Path -LiteralPath $stamp){
    $prior=Get-Content -LiteralPath $stamp -Raw|ConvertFrom-Json
    if(($prior.inputs -join '|') -eq ($inputs -join '|')){
        foreach($file in $prior.files){if((Get-FileHash -LiteralPath (Join-Path $output $file.path)).Hash -ne $file.sha256){throw ('MyGo source cache changed: '+$file.path)}}
        return
    }
}elseif(Test-Path -LiteralPath $output){throw 'Existing MyGo source folder has no generated ownership record; preserving it'}
New-Item -ItemType Directory -Path $output -Force|Out-Null
Get-ChildItem -LiteralPath $module.Dir -Force | Copy-Item -Destination $output -Recurse -Force
foreach($patch in $spec.patches){
    $original=Join-Path $module.Dir $patch.path
    if((Get-FileHash -LiteralPath $original).Hash.ToLowerInvariant() -ne $patch.sha256){throw ('MyGo patch baseline changed: '+$patch.path)}
    $text=[IO.File]::ReadAllText($original)
    foreach($edit in $patch.edits){
        if(([regex]::Matches($text,[regex]::Escape($edit.oldText))).Count -ne 1){throw ('MyGo patch anchor is not unique: '+$patch.path)}
        $text=$text.Replace($edit.oldText,$edit.newText)
    }
    $target=Join-Path $output $patch.path
    (Get-Item -LiteralPath $target).IsReadOnly=$false
    [IO.File]::WriteAllText($target,$text,[Text.UTF8Encoding]::new($false))
}
foreach($addition in $spec.additions){Copy-Item -LiteralPath (Join-Path $source $addition.source) -Destination (Join-Path $output $addition.path) -Force}
$inventory=@(Get-ChildItem -LiteralPath $output -Recurse -File | Where-Object {$_.FullName -ne $stamp} | ForEach-Object {@{path=$_.FullName.Substring($output.Length+1);sha256=(Get-FileHash -LiteralPath $_.FullName).Hash}})
[IO.File]::WriteAllText($stamp,(@{inputs=$inputs;files=$inventory}|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
